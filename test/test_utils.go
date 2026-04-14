package test

import (
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Wait for each listed deployment to enter the "Ready" state.
// Fail the test if one or more deployments are not ready within the timeout.
func waitUntilDeploymentsReady(t *testing.T, options *k8s.KubectlOptions, deployments []string, retries int, sleep time.Duration) {
	var wg sync.WaitGroup
	for _, deploy := range deployments {
		wg.Go(func() {
			k8s.WaitUntilDeploymentAvailable(t, options, deploy, retries, sleep)
		})
	}
	wg.Wait()
}

// Try exec-ing a command in a pod until that command returns a zero exit code.
// Used to poll for "readiness" of a service inside a container
func waitUntilPodExecSucceeds(t *testing.T, options *k8s.KubectlOptions, podName string, command string, retries int, sleep time.Duration) string {
	for range retries {
		res, err := k8s.ExecPodE(t, options, podName, "", "sh", "-c", command)
		if err == nil {
			return res
		}
		t.Logf("Exec '%v' in pod %v failed. Retrying in %v.", command, podName, sleep)
		time.Sleep(sleep)
	}
	t.Fatalf("Exec '%v' in pod %v did not succeed within %v retries", command, podName, retries)
	return ""
}

// Get the name of a pod matching a label selector. Assumes that the label
// uniquely identifies a pod
func getPodNameByLabel(t *testing.T, options *k8s.KubectlOptions, label string) string {
	pods := k8s.ListPods(t, options, v1.ListOptions{LabelSelector: label})
	require.Len(t, pods, 1)
	return pods[0].Name
}
