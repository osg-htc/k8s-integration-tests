// Command loadApparmorProfile loads an AppArmor profile into the host kernel
// (which is shared with the minikube node). Must be run as root.
//
// Usage: loadApparmorProfile <profile-file>
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
	profileFile := os.Args[1]

	name, err := util.AppArmorProfileName(profileFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading profile: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Profile name: %s\n", name)

	util.MustRun("apparmor_parser", "-r", "-W", profileFile)
	if err := exec.Command("grep", "-q", "^"+name+" ", util.AppArmorProfilesPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "profile %q not loaded on host: %v\n", name, err)
		os.Exit(1)
	}
}
