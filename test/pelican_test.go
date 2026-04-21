package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

func TestPelican(t *testing.T) {
	namespace := "test-pelican-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	kustomizeDir := "../manifests/pelican"
	logDir := filepath.Join(LOG_ROOT, filepath.Base(kustomizeDir))
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		t.Logf("Warning: Unable to create log directory %v", logDir)
	}

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)

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
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		th := TestHandle{t, options}
		th.waitUntilAllDeploymentsReady(SIX_MINUTES)
	})
}
