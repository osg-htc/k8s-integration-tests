package test

import (
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
)

type IDTokenOptions struct {
	trustDomain string
	identity    string
	secretName  string
}

type IDTokenData struct {
	passwdManifest string
	tokenManifest  string
}

// Create a random base64-encoded POOL password
func randomPoolPassword(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// applyTemplate executes the Go template at templatePath with the given data,
// returning the rendered result as a string. Fails the test on any error.
func applyTemplate(t *testing.T, templatePath string, data any) string {
	var b strings.Builder
	templ := template.Must(template.ParseFiles(templatePath))
	if err := templ.ExecuteTemplate(&b, filepath.Base(templatePath), data); err != nil {
		t.Fatalf("Failed to template k8s manifest: %v", err)
	}
	return b.String()
}

// generatePoolPasswordAndIDToken creates a random pool password and uses it to generate
// a signed HTCondor IDToken via a temporary pod. Applies the resulting password and token
// as Kubernetes resources and returns their rendered manifests for later cleanup.
func (th *TestHandle) generatePoolPasswordAndIDToken(tokenOptions IDTokenOptions) IDTokenData {
	// Generate a random POOL password
	passwd := randomPoolPassword(16)
	// Create a new K8s template based on the selected tokenOptions and Pool password
	passwdManifest := applyTemplate(th.T, "../manifests/util/generate-idtoken.yaml", map[string]string{
		"trustDomain":  tokenOptions.trustDomain,
		"poolPassword": passwd,
	})

	podname := "idtoken-generator"

	// Create the POOL password secret and a pod that can be used to generate an IDToken
	k8s.KubectlApplyFromString(th.T, th.options, passwdManifest)

	// Wait for the pod to become active
	k8s.WaitUntilPodAvailable(th.T, th.options, podname, 6, 5*time.Second)

	// Generate an IDToken on the pod using `condor_token_create`
	token := k8s.ExecPod(th.T, th.options, podname, "",
		"condor_token_create",
		"-authz", "READ",
		"-authz", "ADVERTISE_STARTD",
		"-authz", "ADVERTISE_MASTER",
		"-identity", tokenOptions.identity)

	tokenManifest := applyTemplate(th.T, "../manifests/util/idtoken.yaml", map[string]string{
		"name":  tokenOptions.secretName,
		"token": token,
	})

	k8s.KubectlApplyFromString(th.T, th.options, tokenManifest)
	k8s.WaitUntilSecretAvailable(th.T, th.options, tokenOptions.secretName, 5, time.Second)

	return IDTokenData{
		passwdManifest: passwdManifest,
		tokenManifest:  tokenManifest,
	}
}

// deletePoolPasswordAndIDToken removes the pool password and IDToken Kubernetes resources
// previously created by generatePoolPasswordAndIDToken.
func (th *TestHandle) deletePoolPasswordAndIDToken(data IDTokenData) {
	k8s.KubectlDeleteFromString(th.T, th.options, data.passwdManifest)
	k8s.KubectlDeleteFromString(th.T, th.options, data.tokenManifest)
}
