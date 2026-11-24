// Command minikubeApparmorPassthrough makes AppArmor profiles loaded on the
// host (see loadApparmorProfile) usable by the kubelet/containerd running
// inside the minikube node. It is a Go translation of the minikube half of
// minikube_apparmor.sh.
//
// Usage: minikubeApparmorPassthrough <profile-file>
package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/osg-htc/k8s-integration-tests/scripts/internal/util"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <profile-file>\n", os.Args[0])
		os.Exit(2)
	}

	name, err := util.AppArmorProfileName(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading profile: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Profile name: %s\n", name)

	// Make AppArmor visible to kubelet/containerd inside the node.
	// The mount may already exist, so failure (and its stderr) is ignored.
	_ = exec.Command("minikube", "ssh", "--", "sudo", "mount", "-t", "securityfs", "securityfs", "/sys/kernel/security").Run()
	util.MustRun("minikube", "ssh", "--", "sudo", "apt-get", "update")
	util.MustRun("minikube", "ssh", "--", "sudo", "apt-get", "install", "-y", "apparmor")

	// Restart so the new setup is picked up
	util.MustRun("minikube", "ssh", "--", "sudo", "systemctl", "restart", "containerd")
	util.MustRun("minikube", "ssh", "--", "sudo", "systemctl", "restart", "kubelet")
	util.MustRun("kubectl", "wait", "--for=condition=Ready", "node", "--all", "--timeout=120s")

	// Verify
	util.MustRun("minikube", "ssh", "--",
		fmt.Sprintf("sudo grep -q '^%s ' %s", name, util.AppArmorProfilesPath))
	util.MustRun("minikube", "ssh", "--", "sudo", "which", "apparmor_parser")
}
