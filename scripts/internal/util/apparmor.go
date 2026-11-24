package util

import (
	"bufio"
	"os"
	"regexp"
)

// AppArmorProfilesPath lists the profiles currently loaded in the kernel.
const AppArmorProfilesPath = "/sys/kernel/security/apparmor/profiles"

var profileLine = regexp.MustCompile(`^profile (\S*) `)

// AppArmorProfileName returns the name from the first "profile <name> ..." line
// of an AppArmor profile file. The name comes from that line, not the filename.
func AppArmorProfileName(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := profileLine.FindStringSubmatch(scanner.Text()); m != nil {
			return m[1], nil
		}
	}
	return "", scanner.Err()
}
