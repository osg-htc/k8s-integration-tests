package test

import (
	"context"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

// subtestGetDataFromOrigin checks that the Pelican CLI tools in the dev pod
// can fetch data from the origin pod
func subtestGetDataFromOrigin(th TestHandle) {
	devPod := th.getPodNameByLabel("app.kubernetes.io/name=dev")
	// Check that condor_status filtered on the EP's name returns a non-empty string
	cmd := "pelican object get pelican://director:8444/public/data/0.0 /dev/null"
	th.waitUntilPodExecSucceeds(devPod, "", cmd, TWO_MINUTES, zeroExitCode)
}

func TestPelican(t *testing.T) {
	namespace := "test-pelican-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	kustomizeDir := "../manifests/pelican"
	logDir := th.makeLogDir(kustomizeDir)

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)

	// bind mount the origin's test data into minikube
	ctx, cancelCtx := context.WithCancel(context.Background())
	th.minikubeBindMount(ctx, "../data/pelican", "/data")

	// TODO OIDC secrets and web UI password are cargo culted from Brian A's repo, their values
	// have no meaning
	secretsManifest := th.applyPelicanSecrets(
		"Placeholder for the registry.",
		"Placeholder for the registry.",
		// Generated using `htpasswd -nbB -C 10 admin asdf`.
		"admin:$2y$10$ONeUS/VGwL9CoAD6pyZ2kusUjX8z0Sxuf8kz2g4PGbFb1GKUQ9J3C")

	k8s.KubectlApplyFromKustomize(t, options, kustomizeDir)

	t.Cleanup(func() {
		th.dumpPodInformation(logDir)
		th.deletePelicanSecrets(secretsManifest)
		k8s.KubectlDeleteFromKustomize(t, options, kustomizeDir)
		k8s.DeleteNamespace(t, options, namespace)
		cancelCtx()
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		th := TestHandle{t, options}
		th.waitUntilAllDeploymentsReady(SIX_MINUTES)
	})

	if t.Failed() {
		return
	}

	t.Run("Confirm public `pelican object get` succeeds", func(t *testing.T) {
		th := TestHandle{t, options}
		subtestGetDataFromOrigin(th)
	})
}
