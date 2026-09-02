package test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

type ospoolEPFormatArgs struct {
	OSPoolEPTag string
	CMTag       string
	CvmfsType   string
}

var defaultOSPoolEPFormatArgs = ospoolEPFormatArgs{
	OSPoolEPTag: "25-release",
	CMTag:       "25.0-el9",
	CvmfsType:   "cvmfsexec",
}

func mkCvmfsMountDir(t *testing.T) string {
	// Volume-mount the relevant host paths into the test Minikube instance
	cvmfsDir, err := os.MkdirTemp("/tmp", "k8s-cvmfs-*")
	if err != nil {
		t.Fatal("Unable to create tmpdir for CMVFS bind-mount")
	}
	err = os.Chmod(cvmfsDir, 0775)
	if err != nil {
		t.Fatal("Unable to set permissons on CMVFS bind-mount dir")
	}
	return cvmfsDir
}

// runOSPoolEPTests runs the set of OSPool EP tests against the EP configuration defined
// in the given kustomizeDir. Note that the `CvmfsType` config var will point the test
// at one of two kustomization directories with different CVMFS mount styles.
func TestOSPoolEP(t *testing.T) {
	t.Parallel()
	kustomizeDir := "../manifests/ospool-ep"

	namespace := "test-ospool-ep-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	// Create a directory for log output
	logDir := th.makeLogDir(kustomizeDir)

	// create k8s namespaces for the test
	k8s.CreateNamespace(t, options, namespace)

	// Volume-mount the relevant host paths into the test Minikube instance
	ctx, cancelCtx := context.WithCancel(context.Background())
	cvmfsDir := mkCvmfsMountDir(t)
	th.minikubeBindMount(ctx, cvmfsDir, "/var/lib/cvmfs-k8s")

	// create the required credentials for cross-container communication in the test
	tokenData := th.generatePoolPasswordAndIDToken("test-cm", "condor@test-cm", "pool-token")

	// Template the kustomize dir
	th.fillTemplateStructFromEnv(&defaultOSPoolEPFormatArgs, "OSPOOL_EP_")
	formattedKustomizeDir := th.formatKustomizeDir(kustomizeDir, defaultOSPoolEPFormatArgs)

	// create k8s resources for the test
	k8s.KubectlApplyFromKustomize(t, options, formattedKustomizeDir)

	// defer deleting the k8s resources created for the test
	t.Cleanup(func() {
		th.dumpPodInformation(logDir)
		cancelCtx()
		k8s.DeleteNamespace(t, options, namespace)
		th.deletePoolPasswordAndIDToken(tokenData)
		k8s.KubectlDeleteFromKustomize(t, options, formattedKustomizeDir)
		os.RemoveAll(cvmfsDir)
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		th := TestHandle{t, options}
		th.waitUntilAllDeploymentsReady(TWO_MINUTES)
	})

	// Bail early here if the deployments do not become live
	if t.Failed() {
		return
	}

	th.RunTestConfigDir("../test-configs/ospool-ep")
}
