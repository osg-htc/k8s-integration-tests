package test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

type pelicanFormatArgs struct {
	Tag         string
	CacheTag    string
	OriginTag   string
	DirectorTag string
	RegistryTag string
	ClientTag   string
}

var defaultPelicanFormatArgs pelicanFormatArgs = pelicanFormatArgs{
	Tag: "v7.26.0",
}

// PelicanTestContext holds all setup components needed to run the Pelican tests, and provides a convenient way to pass them around
type PelicanTestContext struct {
	TestHandle
	logDir                string
	cancelCtx             context.CancelFunc
	secretsManifest       string
	formattedKustomizeDir string
	namespace             string
	kubectlOptions        *k8s.KubectlOptions
}

func setupPelicanTestSpace(t *testing.T) *PelicanTestContext {
	// -----------------------
	// Test environment setup
	// -----------------------

	// Define a test namespace name for the test
	namespace := "test-pelican-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	// Create a logging dir for the test output, fail fast if we can't make it
	kustomizeDir := "../manifests/pelican"
	logDir := th.makeLogDir(kustomizeDir)

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)

	// bind mount the origin's test data into minikube
	ctx, cancelCtx := context.WithCancel(context.Background())
	th.minikubeBindMount(ctx, "../data/pelican", "/data")

	// Create secrets for the pelican services: cert + signing keys
	// TODO OIDC secrets and web UI password are cargo culted from Brian A's repo, their values
	// have no meaning
	secretsManifest := th.applyPelicanSecrets(
		"Placeholder for the registry.",
		"Placeholder for the registry.",
		// Generated using `htpasswd -nbB -C 10 admin asdf`.
		"admin:$2y$10$ONeUS/VGwL9CoAD6pyZ2kusUjX8z0Sxuf8kz2g4PGbFb1GKUQ9J3C")

	// Template the kustomize dir
	th.fillTemplateStructFromEnv(&defaultPelicanFormatArgs, "PELICAN_")
	formattedKustomizeDir := th.formatKustomizeDir(kustomizeDir, defaultPelicanFormatArgs)
	k8s.KubectlApplyFromKustomize(t, options, formattedKustomizeDir)

	return &PelicanTestContext{
		TestHandle:            th,
		logDir:                logDir,
		cancelCtx:             cancelCtx,
		secretsManifest:       secretsManifest,
		formattedKustomizeDir: formattedKustomizeDir,
		namespace:             namespace,
		kubectlOptions:        options,
	}
}

func cleanupPelicanTestSpace(setup *PelicanTestContext) {
	setup.dumpPodInformation(setup.logDir)
	setup.deletePelicanSecrets(setup.secretsManifest)
	k8s.KubectlDeleteFromKustomize(setup.T, setup.kubectlOptions, setup.formattedKustomizeDir)
	k8s.DeleteNamespace(setup.T, setup.kubectlOptions, setup.namespace)
	setup.cancelCtx()
	os.RemoveAll(setup.formattedKustomizeDir)
}

func TestPelican(t *testing.T) {

	testContext := setupPelicanTestSpace(t)

	// --------------------------
	// Test environment teardown
	// --------------------------

	// Cleanup runs all the reciporical functions that delete created resources
	t.Cleanup(func() {
		cleanupPelicanTestSpace(testContext)
	})

	// -------------
	// Actual tests
	// -------------

	// First test: Confirm that the kustomized resources pass their liveness/health checks
	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		testContext.waitUntilAllDeploymentsReady(SIX_MINUTES)
	})

	if t.Failed() {
		return
	}

	// Second test: Run a basic pelican object get
	testContext.RunTestConfigDir("../test-configs/pelican")
}
