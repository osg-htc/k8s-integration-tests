package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"

	"github.com/stretchr/testify/require"
)

// TWO_MINUTES is the default retry configuration for polling tests
var TWO_MINUTES = Retry{12, 10 * time.Second}

// SIX_MINUTES is a longer timeout for tests that take a long time such as CVMFS tests
var SIX_MINUTES = Retry{12, 30 * time.Second}

var LOG_ROOT = "/tmp/k8s-tests"

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
	th.waitUntilPodExecSucceeds(cmPod, "", cmd, SIX_MINUTES, truthy)
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
	tokenData := th.generatePoolPasswordAndIDToken("test-cm", "condor@test-cm", "pool-token")
	// create k8s resources for the test
	k8s.KubectlApplyFromKustomize(t, options, resourcePath)

	// Create a directory for log output
	logDir := filepath.Join(LOG_ROOT, filepath.Base(kustomizeDir))
	err = os.MkdirAll(logDir, 0755)
	if err != nil {
		t.Logf("Warning: Unable to create log directory %v", logDir)
	}
	// defer deleting the k8s resources created for the test
	t.Cleanup(func() {
		th.dumpPodInformation(logDir)
		k8s.DeleteNamespace(t, options, namespace)
		th.deletePoolPasswordAndIDToken(tokenData)
		k8s.KubectlDeleteFromKustomize(t, options, resourcePath)
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		th := TestHandle{t, options}
		th.waitUntilAllDeploymentsReady(TWO_MINUTES)
	})

	// Bail early here if the deployments do not become live
	if t.Failed() {
		return
	}

	t.Run("Confirm condor_status lists the EP.", func(t *testing.T) {
		t.Parallel()
		subtestCondorStatus(TestHandle{t, options})
	})

	t.Run("Confirm EP container advertises singularity.", func(t *testing.T) {
		t.Parallel()
		subtestHasSingularity(TestHandle{t, options})
	})

	t.Run("Confirm EP container advertises CVMFS", func(t *testing.T) {
		t.Parallel()
		subtestHasCVMFS(TestHandle{t, options})
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
