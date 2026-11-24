package util

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run executes a command with stdout/stderr passed through, returning any error.
func Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// MustRun is like Run but exits the process on failure (equivalent to `set -e`).
func MustRun(name string, args ...string) {
	if err := Run(name, args...); err != nil {
		fmt.Fprintf(os.Stderr, "command failed: %s %s: %v\n", name, strings.Join(args, " "), err)
		os.Exit(1)
	}
}
