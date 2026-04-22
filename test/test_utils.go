package test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LOG_ROOT is the top level logging directory for dumping test results
var LOG_ROOT = "/tmp/k8s-tests"

// TWO_MINUTES is the default retry configuration for polling tests
var TWO_MINUTES = Retry{12, 10 * time.Second}

// SIX_MINUTES is a longer timeout for tests that take a longer time
var SIX_MINUTES = Retry{12, 30 * time.Second}

// Wrapper for common args passed to every test function
type TestHandle struct {
	*testing.T
	options *k8s.KubectlOptions
}

// Wrapper for the common (retry count, retry delay) result polling construct
type Retry struct {
	retries int
	sleep   time.Duration
}

// truthy returns whether a string evaluates to "True", "true", "TRUE", etc.
func truthy(s string) bool {
	return strings.ToLower(strings.TrimSpace(s)) == "true"
}

// nonEmpty returns whether a string has content
func nonEmpty(s string) bool {
	return len(s) > 0
}

// zeroExitCode returns true as long as the command exits with code 0, regardless
// of its output
func zeroExitCode(_ string) bool {
	return true
}

// waitUntilDeploymentsReady waits for each listed deployment to enter the "Ready" state.
// Fail the test if one or more deployments are not ready within the timeout.
func (th *TestHandle) waitUntilDeploymentsReady(deployments []string, retries Retry) {
	var wg sync.WaitGroup
	for _, deploy := range deployments {
		wg.Go(func() {
			k8s.WaitUntilDeploymentAvailable(th.T, th.options, deploy, retries.retries, retries.sleep)
		})
	}
	wg.Wait()
}

// waitUntilAllDeploymentsReady waits until all deployments in the given namespace
// enter the "Ready" state. Fail the test if one or more deployments are not
// ready within the timeout.
func (th *TestHandle) waitUntilAllDeploymentsReady(retries Retry) {
	allDeploys := k8s.ListDeployments(th.T, th.options, v1.ListOptions{})
	deployments := make([]string, 0)
	for _, deploy := range allDeploys {
		deployments = append(deployments, deploy.Name)
	}
	th.waitUntilDeploymentsReady(deployments, retries)
}

type EvalOutput func(string) bool

// waitUntilPodExecSucceeds tries exec-ing a command in a pod until that command both returns a zero exit code and passes the provided
// evaluation expression. Used to poll for "readiness" of a service inside a container
func (th *TestHandle) waitUntilPodExecSucceeds(podName string, containerName string, command string, retries Retry, evaluator EvalOutput) string {
	for range retries.retries {
		res, err := k8s.ExecPodE(th.T, th.options, podName, containerName, "sh", "-c", command)
		if err == nil && evaluator(res) {
			return res
		}
		th.T.Logf("Exec '%v' in pod %v failed. Retrying in %v.", command, podName, retries.sleep)
		time.Sleep(retries.sleep)
	}
	th.T.Fatalf("Exec '%v' in pod %v did not succeed within %v retries", command, podName, retries)
	return ""
}

// getPodNameByLabel gets the name of a pod matching a label selector. Assumes that the label
// uniquely identifies a pod
func (th *TestHandle) getPodNameByLabel(label string) string {
	th.T.Helper()
	pods := k8s.ListPods(th.T, th.options, v1.ListOptions{LabelSelector: label})
	require.Len(th.T, pods, 1)
	return pods[0].Name
}

// eventTimestamp returns the most precise available timestamp for a core/v1 Event.
// core/v1 events use LastTimestamp; EventTime is only set for events.k8s.io/v1.
func eventTimestamp(e corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	return e.EventTime.Time
}

// dumpPodEvents returns a string containing all observed events for a pod
// in timestamp order
func (th *TestHandle) dumpPodEvents(podName string, outputDir string) (eventsLog string, err error) {
	th.T.Helper()
	fieldSelector := fmt.Sprintf("involvedObject.name=%v", podName)
	events, err := k8s.ListEventsE(th.T, th.options, v1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		return
	}

	slices.SortFunc(events, func(a, b corev1.Event) int {
		return eventTimestamp(a).Compare(eventTimestamp(b))
	})

	var sb strings.Builder
	for _, event := range events {
		fmt.Fprintf(&sb, "%v\t%v\t%v\n", eventTimestamp(event), event.Type, event.Message)
	}

	eventsLog = sb.String()
	path := filepath.Join(outputDir, fmt.Sprintf("%v.events", podName))

	err = os.WriteFile(path, []byte(eventsLog), 0644)
	if err != nil {
		return
	}

	return
}

// dumpPodLogs iterates over containers in the given pod, then dumps the logs from
// each container into the specified outputDir
func (th *TestHandle) dumpPodLogs(podName string, outputDir string) (err error) {
	th.T.Helper()
	pod, err := k8s.GetPodE(th.T, th.options, podName)
	if err != nil {
		return
	}
	containers := make([]string, 0)
	for _, container := range pod.Spec.Containers {
		containers = append(containers, container.Name)
	}

	for _, containerName := range containers {
		logs, err2 := k8s.GetPodLogsE(th.T, th.options, pod, containerName)
		if err2 != nil {
			return err2
		}
		path := filepath.Join(outputDir, fmt.Sprintf("%v_%v.log", podName, containerName))

		err = os.WriteFile(path, []byte(logs), 0644)
		if err != nil {
			return err
		}
	}
	return
}

// dumpPodInformation dumps pod events upon test completion
func (th *TestHandle) dumpPodInformation(logDir string) {
	th.T.Helper()
	pods := k8s.ListPods(th.T, th.options, v1.ListOptions{})
	// First, export all pod logs as build artifacts
	for _, pod := range pods {
		err := th.dumpPodLogs(pod.Name, logDir)
		if err != nil {
			th.T.Logf("Unable to export logs for pod %v: %v", pod.Name, err)
		}
	}

	// then, dump pod events
	for _, pod := range pods {
		events, err := th.dumpPodEvents(pod.Name, logDir)
		if err != nil {
			th.T.Logf("Unable to get events for pod %v: %v", pod, err)
		}
		th.T.Logf("---\nEvents for pod %v:\n%v\n---", pod.Name, events)
	}
}

// makeLogDir creates a temporary directory for storing Pod logs and events
func (th *TestHandle) makeLogDir(kustomizeDir string) string {
	th.T.Helper()
	logDir := filepath.Join(LOG_ROOT, filepath.Base(kustomizeDir))
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		th.T.Fatalf("Warning: Unable to create log directory %v for storing test results. Failing.", logDir)
	}
	return logDir
}

// minikubeBindMount forks a `minikube mount` process to the given hostDir
// to enable access to files on the host from inside the cluster
func (th *TestHandle) minikubeBindMount(ctx context.Context, hostDir string, destDir string) *exec.Cmd {
	th.T.Helper()
	cmd := exec.CommandContext(ctx, "minikube", "mount", fmt.Sprintf("%v:%v", hostDir, destDir))
	err := cmd.Start()
	if err != nil {
		th.T.Fatalf("Unable to start a minikube bindmount from %v to %v. Failing", hostDir, destDir)
	}
	return cmd
}

// formatKustomizeDir applies the given format struct to all go-templated yaml files in the
// given kustomize dir into a temporary output directory
func (th *TestHandle) formatKustomizeDir(kustomizeDir string, formatArgs any) string {
	th.T.Helper()
	newKustomizeDir, err := os.MkdirTemp("/tmp", "kustomize-template-*")
	if err != nil {
		th.T.Fatal("Unable to create temporary directory for formatted kustomize.")
	}

	err = filepath.WalkDir(kustomizeDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		// Template the source file based on the given formatArgs
		formatted := applySprigTemplate(th.T, path, formatArgs)
		relPath, err := filepath.Rel(kustomizeDir, path)
		if err != nil {
			return err
		}

		// Create a new ouput path for the templated file relative to the
		// template base
		newPath := filepath.Join(newKustomizeDir, relPath)
		dirName := filepath.Dir(newPath)
		if err := os.MkdirAll(dirName, 0755); err != nil {
			return err
		}

		// Write the formatted copy of the file to its new destination
		err = os.WriteFile(newPath, []byte(formatted), 0644)
		return err
	})
	if err != nil {
		th.T.Fatalf("Unable to format kustomization files: %v.", err)
	}
	return newKustomizeDir
}

// fillTemplateStructFromEnv populates the value of a struct based on prefixed environemnt variables
// for example, given namePrefix = "PELICAN_", this will set the `Tag` field of the given struct
// to the value of `PELICAN_Tag`
func (th *TestHandle) fillTemplateStructFromEnv(tStruct any, namePrefix string) {
	th.T.Helper()
	rv := reflect.ValueOf(tStruct)

	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		th.Fatalf("Cannot set fields for %v based on env", tStruct)
	}
	for structField, structVal := range rv.Elem().Fields() {
		if !structVal.CanSet() {
			continue
		}
		expectedEnv := fmt.Sprintf("%v%v", namePrefix, structField.Name)
		if envVal, exists := os.LookupEnv(expectedEnv); exists {
			structVal.Set(reflect.ValueOf(envVal))
		}
	}
}
