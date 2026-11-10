package test

import (
	"fmt"
	"os/exec"
)

// buildMinikubeImage builds the Dockerfile in dockerDir directly into minikube's
// own image store via `minikube image build`, tagging the result as tag.
// buildArgs are passed through as Docker build args.
func (th *TestHandle) buildMinikubeImage(dockerDir string, tag string, buildArgs map[string]string) {
	th.T.Helper()
	args := []string{"image", "build", "-t", tag}
	for k, v := range buildArgs {
		args = append(args, "--build-opt", fmt.Sprintf("opt=build-arg:%s=%s", k, v))
	}
	args = append(args, dockerDir)

	cmd := exec.Command("minikube", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		th.T.Fatalf("Failed to build minikube image %v: %v\n%v", tag, err, string(out))
	}
	th.T.Logf("Built minikube image %v:\n%v", tag, string(out))
}
