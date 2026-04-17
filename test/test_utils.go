package test

import (
	"fmt"
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

// getPodEvents returns a string containing all observed events for a pod
// in timestamp order
func (th *TestHandle) getPodEvents(podName string) string {
	fieldSelector := fmt.Sprintf("involvedObject.name=%v", podName)
	events := k8s.ListEvents(th.T, th.options, v1.ListOptions{FieldSelector: fieldSelector})

	slices.SortFunc(events, func(a, b corev1.Event) int {
		return eventTimestamp(a).Compare(eventTimestamp(b))
	})

	var sb strings.Builder
	for _, event := range events {
		fmt.Fprintf(&sb, "%v\t%v\t%v\n", eventTimestamp(event), event.Type, event.Message)
	}
	return sb.String()
}
