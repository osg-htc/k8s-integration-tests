package test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
)

// adstashRunnerImage is the tag prefix for the test-runner image built by
// TestAdstash into minikube (see buildMinikubeImage in test_runner_utils.go).
const adstashRunnerImage = "condor-adstash-tests"

// adstashFormatArgs parameterizes the search-engine backend under test, the
// HTCondor image, and the test-runner image (which bakes in a specific
// elasticsearch-py/opensearch-py combo at build time).
type adstashFormatArgs struct {
	SEBackendType string // elasticsearch | opensearch — selects the search-engine image family
	DBTag         string // full search-engine image tag, e.g. "8.19.20"
	DBMajorTag    string // major version parsed from DBTag, e.g. "8" — selects version-specific env vars
	CondorTag     string // htcondor/mini image tag
	RunnerTag     string // test-runner image tag (encodes the ES/OS client-lib combo baked into it)
	SEPyVersion   string // pip version constraint for the search library (es or os), baked into the runner image at build time
}

var defaultAdstashFormatArgs = adstashFormatArgs{
	SEBackendType: "elasticsearch",
	DBTag:         "8.19.20",
	CondorTag:     "lts",
	RunnerTag:     "es7py-os2py",
	SEPyVersion:   "elasticsearch>=7,<8",
}

// TestAdstash runs the adstash test suite against the search-engine backend
// configured in defaultAdstashFormatArgs (overridable via ADSTASH_* env vars).
func TestAdstash(t *testing.T) {
	t.Parallel()
	kustomizeDir := "../manifests/adstash"

	namespace := "test-adstash-" + strings.ToLower(random.UniqueId())
	options := k8s.NewKubectlOptions("", "", namespace)
	th := TestHandle{t, options}

	// Populate the test params struct from the environment
	th.fillTemplateStructFromEnv(&defaultAdstashFormatArgs, "ADSTASH_")

	// Build the test-runner image into minikube
	th.buildMinikubeImage("../images/adstash",
		fmt.Sprintf("%v:%v", adstashRunnerImage, defaultAdstashFormatArgs.RunnerTag),
		map[string]string{
			"SE_PY_VERSION": defaultAdstashFormatArgs.SEPyVersion,
		})

	// Create a directory for log output
	logDir := th.makeLogDir(kustomizeDir)

	// create k8s namespace for the test
	k8s.CreateNamespace(t, options, namespace)

	// Derive the major-version tag (e.g. "8") from the full DBTag (e.g. "8.19.20")
	// used to select version-specific env vars in the rendered manifests.
	defaultAdstashFormatArgs.DBMajorTag = strings.SplitN(defaultAdstashFormatArgs.DBTag, ".", 2)[0]

	// create the pool password and IDToken the runner uses to authenticate to the schedd
	tokenData := th.generatePoolPasswordAndIDToken("condor", "submituser@condor", "adstash-pool-token",
		[]string{"READ", "WRITE"})

	formattedKustomizeDir := th.formatKustomizeDir(kustomizeDir, defaultAdstashFormatArgs)

	// create k8s resources for the test
	k8s.KubectlApplyFromKustomize(t, options, formattedKustomizeDir)

	// defer deleting the k8s resources created for the test
	t.Cleanup(func() {
		th.dumpPodInformation(logDir)
		k8s.DeleteNamespace(t, options, namespace)
		th.deletePoolPasswordAndIDToken(tokenData)
		k8s.KubectlDeleteFromKustomize(t, options, formattedKustomizeDir)
		os.RemoveAll(formattedKustomizeDir)
	})

	t.Run("Confirm deployments become ready.", func(t *testing.T) {
		th := TestHandle{t, options}
		th.waitUntilAllDeploymentsReady(SIX_MINUTES)
	})

	// Bail early here if the deployments do not become live
	if t.Failed() {
		return
	}

	th.RunTestConfigDir("../test-configs/adstash")
}
