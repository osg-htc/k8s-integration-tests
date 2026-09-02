package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/labels"
)

// podSelectorConfig identifies the pod a scripted test should run against.
type podSelectorConfig struct {
	Labels map[string]string `yaml:"labels"`
}

// retryConfigYAML is the (timeout, wait) pair, in seconds, read from a test's `retry` field.
type retryConfigYAML struct {
	Timeout int `yaml:"timeout"`
	Wait    int `yaml:"wait"`
}

// scriptedTestConfig is a single entry under a testConfig.yaml's `tests:` list.
// Exactly one of Script or Command should be set: Script is copied into the pod and
// exec'd via `sh -c`, while Command is exec'd directly, which is required for pods
// (e.g. distroless containers) that don't have a shell.
type scriptedTestConfig struct {
	Name        string            `yaml:"name"`
	PodSelector podSelectorConfig `yaml:"podSelector"`
	Script      string            `yaml:"script"`
	Command     []string          `yaml:"command"`
	Container   string            `yaml:"container"`
	Retry       retryConfigYAML   `yaml:"retry"`
}

// testConfigFile is the top-level shape of a testConfig.yaml file.
type testConfigFile struct {
	Tests []scriptedTestConfig `yaml:"tests"`
}

// toRetry converts a (timeout, wait) pair given in seconds into the (retries, sleep)
// form used by waitUntilPodExecSucceeds.
func (r retryConfigYAML) toRetry() Retry {
	wait := time.Duration(r.Wait) * time.Second
	retries := (r.Timeout + r.Wait - 1) / r.Wait
	return Retry{retries, wait}
}

// labelSelector renders a podSelector's labels as a Kubernetes label selector string.
func (p podSelectorConfig) labelSelector() string {
	return labels.Set(p.Labels).String()
}

// loadTestConfigFile parses the testConfig.yaml file found in configDir, failing the
// test if the file does not exist or cannot be parsed.
func loadTestConfigFile(t *testing.T, configDir string) testConfigFile {
	t.Helper()
	configPath := filepath.Join(configDir, "testConfig.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Unable to read test config file %v: %v", configPath, err)
	}

	var config testConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("Unable to parse test config file %v: %v", configPath, err)
	}

	for _, cfg := range config.Tests {
		if cfg.Script != "" && len(cfg.Command) > 0 {
			t.Fatalf("Test %q in %v specifies both script and command; only one is allowed", cfg.Name, configPath)
		}
	}

	return config
}

// runScriptedTest runs cfg's command, or, if no command is given, copies cfg's script
// into the pod matched by its podSelector and makes it executable. Either way, it then
// retries executing the command/script until it exits 0.
func (th TestHandle) runScriptedTest(configDir string, cfg scriptedTestConfig) {
	th.T.Helper()
	podName := th.getPodNameByLabel(cfg.PodSelector.labelSelector())
	retry := cfg.Retry.toRetry()

	if len(cfg.Command) > 0 {
		th.waitUntilPodExecSucceedsSlice(podName, cfg.Container, cfg.Command, retry, zeroExitCode)
		return
	}

	localScript := filepath.Join(configDir, cfg.Script)
	remoteScript := fmt.Sprintf("/tmp/%v", filepath.Base(cfg.Script))

	cpArgs := []string{"cp", localScript, fmt.Sprintf("%v:%v", podName, remoteScript)}
	if cfg.Container != "" {
		cpArgs = append(cpArgs, "-c", cfg.Container)
	}
	k8s.RunKubectl(th.T, th.options, cpArgs...)

	k8s.ExecPod(th.T, th.options, podName, cfg.Container, "chmod", "+x", remoteScript)

	th.waitUntilPodExecSucceeds(podName, cfg.Container, remoteScript, retry, zeroExitCode)
}

// RunTestConfigDir parses the testConfig.yaml file in configDir and runs each of its
// `tests:` entries as a subtest of th.T.
func (th TestHandle) RunTestConfigDir(configDir string) {
	th.T.Helper()
	config := loadTestConfigFile(th.T, configDir)

	for _, cfg := range config.Tests {
		th.T.Run(cfg.Name, func(t *testing.T) {
			TestHandle{t, th.options}.runScriptedTest(configDir, cfg)
		})
	}
}
