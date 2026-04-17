package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"
)

// TWO_MINUTES is the default retry configuration for polling tests
var TWO_MINUTES = Retry{10, 30 * time.Second}

// TEN_MINUTES is a longer timeout for tests that take a long time such as CVMFS tests
var TEN_MINUTES = Retry{20, 30 * time.Second}

// Check that all deployments in the namespace become "ready" within 2 minutes
func subtestDeploymentsReady(th TestHandle) {
	deployments := []string{"test-cm", "ospool-ep"}
	th.waitUntilDeploymentsReady(deployments, TWO_MINUTES)
}

// Check that condor_status run against the CM lists the EP
func subtestCondorStatus(th TestHandle) {
	cmPod := th.getPodNameByLabel("app=test-cm")
	epPod := th.getPodNameByLabel("app=ospool-ep")
	// Check that condor_status filtered on the EP's name returns a non-empty string
	cmd := fmt.Sprintf(`condor_status -const 'regexp("%v",Machine)'`, epPod)
	th.waitUntilPodExecSucceeds(cmPod, "", cmd, TWO_MINUTES, nonEmpty)
}

// Check that the EP advertises that it can run Apptainer
func subtestHasSingularity(th TestHandle) {
	cmPod := th.getPodNameByLabel("app=test-cm")
	epPod := th.getPodNameByLabel("app=ospool-ep")
	// Check that condor_status filtered on the EP's name returns a non-empty string
	cmd := fmt.Sprintf(`condor_status -const 'regexp("%v",Machine)' -af HAS_SINGULARITY`, epPod)
	th.waitUntilPodExecSucceeds(cmPod, "", cmd, TWO_MINUTES, truthy)
}

// Check that the EP advertises the two test CVMFS repos
func subtestHasCVMFS(th TestHandle) {
	cmPod := th.getPodNameByLabel("app=test-cm")
	epPod := th.getPodNameByLabel("app=ospool-ep")

	cvmfsAd := "HAS_CVMFS_singularity_opensciencegrid_org"
	cmd := fmt.Sprintf(`condor_status -const 'regexp("%v",Machine)' -af %v`, epPod, cvmfsAd)
	th.waitUntilPodExecSucceeds(cmPod, "", cmd, TEN_MINUTES, truthy)
}

// runOSPoolEPTests runs the set of OSPool EP tests against the EP configuration defined
// in the given kustomizeDir
func runOSPoolEPTests(t *testing.T, kustomizeDir string) {
	resourcePath, err := filepath.Abs(kustomizeDir)
	require.NoError(t, err)

	namespace := "test-ospool-ep-" + strings.ToLower(random.UniqueId())

	options := k8s.NewKubectlOptions("", "", namespace)

	th := TestHandle{t, options}

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)
	// create the required credentials for cross-container communication in the test
	tokenData := th.generatePoolPasswordAndIDToken(IDTokenOptions{
		trustDomain: "test-cm",
		identity:    "condor@test-cm",
		secretName:  "pool-token",
	})
	// create k8s resources for the test
	k8s.KubectlApplyFromKustomize(t, options, resourcePath)

	// defer deleting the k8s resources created for the test
	t.Cleanup(func() {
		k8s.DeleteNamespace(t, options, namespace)
		th.deletePoolPasswordAndIDToken(tokenData)
		k8s.KubectlDeleteFromKustomize(t, options, resourcePath)
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		subtestDeploymentsReady(TestHandle{t, options})
	})

	t.Run("Confirm condor_status lists the EP.", func(t *testing.T) {
		t.Parallel()
		th := TestHandle{t, options}
		t.Cleanup(func() { dumpDebugLogs(th) })
		subtestCondorStatus(th)
	})

	t.Run("Confirm EP container advertises singularity.", func(t *testing.T) {
		t.Parallel()
		th := TestHandle{t, options}
		t.Cleanup(func() { dumpDebugLogs(th) })
		subtestHasSingularity(th)
	})

	t.Run("Confirm EP container advertises CVMFS", func(t *testing.T) {
		t.Parallel()
		th := TestHandle{t, options}
		t.Cleanup(func() { dumpDebugLogs(th) })
		subtestHasCVMFS(th)
	})

}

// TestOSPoolEPCvmfsexec is an entrypoint test for testing an EP configured
// with CVMFSExec
func TestOSPoolEPCvmfsexec(t *testing.T) {
	t.Parallel()
	runOSPoolEPTests(t, "../manifests/ospool-ep-cvmfsexec")
}

// TestOSPoolEPCvmfsexec is an entrypoint test for testing an EP configured
// with CVMFS bind mounts
func TestOSPoolEPCvmfsBindMount(t *testing.T) {
	t.Parallel()
	runOSPoolEPTests(t, "../manifests/ospool-ep-cvmfs-bind")
}

// dumpDebugLogs dumps pod events and logs upon test completion
func dumpDebugLogs(th TestHandle) {
	epPodName := th.getPodNameByLabel("app=ospool-ep")
	// epPod := k8s.GetPod(th.T, th.options, epPodName)
	// logs := k8s.GetPodLogs(th.T, th.options, epPod, "")
	fieldSelector := fmt.Sprintf("involvedObject.name=%v", epPodName)
	events := k8s.ListEvents(th.T, th.options, v1.ListOptions{FieldSelector: fieldSelector})

	// th.T.Logf("Logs for pod %v:\n%v", epPodName, logs)
	var sb strings.Builder
	for _, event := range events {
		fmt.Fprintf(&sb, "%v\t%v\t%v\n", event.EventTime, event.Type, event.Message)
	}
	th.T.Logf("Events for pod %v:\n%v", epPodName, sb.String())
}
