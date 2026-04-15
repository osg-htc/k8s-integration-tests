package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"

	"github.com/stretchr/testify/require"
)

// TWO_MINUTES is the default retry configuration for polling tests
var TWO_MINUTES = Retry{12, 10 * time.Second}

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

	cvmfsAds := []string{"HAS_CVMFS_singularity_opensciencegrid_org", "HAS_CVMFS_oasis_opensciencegrid_org"}
	var wg sync.WaitGroup
	for _, ad := range cvmfsAds {
		cmd := fmt.Sprintf(`condor_status -const 'regexp("%v",Machine)' -af %v`, epPod, ad)
		wg.Go(func() {
			th.waitUntilPodExecSucceeds(cmPod, "", cmd, TWO_MINUTES, truthy)
		})
	}
	wg.Wait()
}

// Entrypoint test: Creates a fresh namespace, applies a kustomization
// that creates an OSPool EP and a CM, then runs sub-tests
func TestOSPoolEP(t *testing.T) {
	t.Parallel()

	resourcePath, err := filepath.Abs("../manifests/ospool-ep")
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
	defer k8s.DeleteNamespace(t, options, namespace)
	defer th.deletePoolPasswordAndIDToken(tokenData)
	defer k8s.KubectlDeleteFromKustomize(t, options, resourcePath)

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		subtestDeploymentsReady(TestHandle{t, options})
	})

	t.Run("Confirm condor_status lists the EP.", func(t *testing.T) {
		subtestCondorStatus(TestHandle{t, options})
	})

	t.Run("Confirm EP container advertises singularity.", func(t *testing.T) {
		subtestHasSingularity(TestHandle{t, options})
	})

	t.Run("Confirm EP container advertises CVMFS", func(t *testing.T) {
		subtestHasCVMFS(TestHandle{t, options})
	})
}
