package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"

	//"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/require"
)

// Check that all deployments in the namespace become "ready" within 2 minutes
func subtestDeploymentsReady(t *testing.T, options *k8s.KubectlOptions) {
	deployments := []string{"test-cm", "ospool-ep"}
	waitUntilDeploymentsReady(t, options, deployments, 12, 10*time.Second)
}

// Check that condor_status run against the CM lists the EP
func subtestCondorStatus(t *testing.T, options *k8s.KubectlOptions) {
	cmPod := getPodNameByLabel(t, options, "app=test-cm")
	epPod := getPodNameByLabel(t, options, "app=ospool-ep")
	// Check that condor_status filtered on the EP's name returns a non-empty string
	cmd := fmt.Sprintf("[ -n \"$(condor_status -const 'regexp(\"%v\",Machine)')\" ]", epPod)
	waitUntilPodExecSucceeds(t, options, cmPod, cmd, 12, 10*time.Second)
}

// Entrypoint test: Creates a fresh namespace, applies a kustomization
// that creates an OSPool EP and a CM, then runs sub-tests
func TestOSPoolEP(t *testing.T) {
	t.Parallel()

	resourcePath, err := filepath.Abs("../manifests/ospool-ep")
	require.NoError(t, err)

	namespace := "test-ospool-ep-" + strings.ToLower(random.UniqueId())

	options := k8s.NewKubectlOptions("", "", namespace)

	// defer deleting the k8s resources created for the test
	defer k8s.DeleteNamespace(t, options, namespace)
	defer k8s.KubectlDeleteFromKustomize(t, options, resourcePath)

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)
	// create k8s resources for the test
	k8s.KubectlApplyFromKustomize(t, options, resourcePath)

	// Wait for each expected deployment to enter the "ready" state
	// before running tests
	t.Run("Confirm deployments become ready.", func(t *testing.T) { subtestDeploymentsReady(t, options) })

	// Very basic proof of concept: confirm that `condor_status` on the CM contains the
	// AP's pod name
	t.Run("Confirm condor_status lists the EP", func(t *testing.T) { subtestCondorStatus(t, options) })
}
