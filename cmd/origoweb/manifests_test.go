// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package main

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// numericUser is the USER line of an image a Kubernetes pod may ask to run
// as non-root: two ids, no names.
var numericUser = regexp.MustCompile(`(?m)^USER \d+:\d+$`)

// TestTheImageAndThePodAgreeOnANumericUser is the criterion a name cost a
// failed rollout: a pod with runAsNonRoot never starts against an image
// whose USER is a name, because the kubelet resolves no names and answers
// CreateContainerConfigError. So the Dockerfile writes the id and the base
// Deployment asks for the same one.
func TestTheImageAndThePodAgreeOnANumericUser(t *testing.T) {
	root := moduleRoot(t)
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	dockerfile := read("Dockerfile")
	if !numericUser.MatchString(dockerfile) {
		t.Errorf("the Dockerfile has no numeric USER line:\n%s", dockerfile)
	}
	if strings.Contains(dockerfile, "USER nonroot") {
		t.Error("the Dockerfile names the user rather than numbering it")
	}

	deployment := read("deploy/base/deployment.yaml")
	for _, want := range []string{"runAsNonRoot: true", "runAsUser: 65532", "runAsGroup: 65532"} {
		if !strings.Contains(deployment, want) {
			t.Errorf("deploy/base/deployment.yaml does not carry %q", want)
		}
	}
	if !strings.Contains(dockerfile, "USER 65532:65532") {
		t.Error("the Dockerfile's USER is not the 65532 the Deployment asks for")
	}
}

// moduleRoot walks up from the working directory to the module root, so a
// test reads a repository file whatever directory `go test` ran it in.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// TestTheInterfaceAsksOnlyForTheOIDCMinimum pins the scope set the browser
// flow requests. fosite refuses an authorization request naming a scope the
// client row does not hold, so a scope that outlives its grant is a sign-in
// that does not work rather than a feature that is absent: auth drops
// `origo:ssh-keys` from the `origoweb` client now that key management has
// moved to the platform control plane, which authorizes with the
// `api.latere.ai` actor token and no login scope of its own.
//
// The merged set is what matters, not either file: kustomize keys `env` by
// name, so the production overlay's entry replaces the base's. An exact set
// also fails when the setting vanishes altogether, which would drop
// offline_access and kill the session at the first token expiry.
func TestTheInterfaceAsksOnlyForTheOIDCMinimum(t *testing.T) {
	root := moduleRoot(t)
	env := map[string]string{}
	for _, name := range []string{"deploy/base/deployment.yaml", "deploy/prod/settings.yaml"} {
		maps.Copy(env, containerEnv(t, filepath.Join(root, name)))
	}

	raw, ok := env["ORIGOWEB_AUTH_SCOPES"]
	if !ok {
		t.Fatal("no manifest sets ORIGOWEB_AUTH_SCOPES")
	}
	got := map[string]bool{}
	for scope := range strings.SplitSeq(raw, ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			got[scope] = true
		}
	}
	want := map[string]bool{"openid": true, "email": true, "profile": true, "offline_access": true}
	for scope := range got {
		if !want[scope] {
			t.Errorf("the interface requests %q, which the issuer does not grant this client", scope)
		}
	}
	for scope := range want {
		if !got[scope] {
			t.Errorf("the interface no longer requests %q", scope)
		}
	}
}

// containerEnv reads the origoweb container's environment out of a
// Deployment or of a strategic-merge patch shaped like one. Only entries
// with a literal value are returned: a secretKeyRef names no value here.
func containerEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Name string `yaml:"name"`
						Env  []struct {
							Name  string `yaml:"name"`
							Value string `yaml:"value"`
						} `yaml:"env"`
					} `yaml:"containers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, container := range manifest.Spec.Template.Spec.Containers {
		if container.Name != "origoweb" {
			continue
		}
		for _, entry := range container.Env {
			if entry.Value != "" {
				env[entry.Name] = entry.Value
			}
		}
	}
	if len(env) == 0 {
		t.Fatalf("%s sets no environment on the origoweb container", path)
	}
	return env
}
