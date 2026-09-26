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

// imageLine is a container's image reference in a manifest, in block style,
// as the first key of a list item or as a key of its own. A commented line
// starts with `#` and does not match.
var imageLine = regexp.MustCompile(`(?m)^\s*(?:-\s+)?image:\s*(\S+)`)

// TestNoManifestNamesAMovingOrMissingTag holds every manifest under deploy/
// to a tag that names something. An overlay that pins no release runs the
// base's tag, and the release workflow publishes only vX.Y.Z tags: a
// `latest` in the base is a reference nothing publishes that reads as the
// newest release, and an image with no tag is the same reference, because
// the runtime pulls `latest` for it. The base carries the placeholder
// `unreleased` instead, which is never published either, so the failed pull
// names its own cause.
func TestNoManifestNamesAMovingOrMissingTag(t *testing.T) {
	root := moduleRoot(t)
	found := 0
	err := filepath.WalkDir(filepath.Join(root, "deploy"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, m := range imageLine.FindAllStringSubmatch(string(body), -1) {
			found++
			if problem := imageTagProblem(m[1]); problem != "" {
				t.Errorf("%s names %s, %s", rel, m[1], problem)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("no manifest under deploy/ names an image, so this test pins nothing")
	}
}

// imageTagProblem says what is wrong with an image reference's tag, or
// returns the empty string when the reference pins one. A digest pins the
// bytes whatever the tag says. The tag follows the last colon after the last
// slash, so a registry port is not read as one.
func imageTagProblem(ref string) string {
	ref = strings.Trim(ref, `"'`)
	if strings.Contains(ref, "@") {
		return ""
	}
	name := ref[strings.LastIndex(ref, "/")+1:]
	i := strings.LastIndex(name, ":")
	switch {
	case i < 0 || i == len(name)-1:
		return "which has no tag and pulls `latest`"
	case name[i+1:] == "latest":
		return "and no `latest` tag is published"
	}
	return ""
}

// TestTheTagRuleRefusesLatestAndNoTag holds the rule to the references it
// exists to catch, so a parser that read every reference as pinned would not
// leave the manifest test passing.
func TestTheTagRuleRefusesLatestAndNoTag(t *testing.T) {
	for ref, pinned := range map[string]bool{
		"ghcr.io/latere-ai/origoweb:latest":           false,
		"ghcr.io/latere-ai/origoweb":                  false,
		"ghcr.io/latere-ai/origoweb:":                 false,
		"localhost:5000/origoweb":                     false,
		`"ghcr.io/latere-ai/origoweb:latest"`:         false,
		"ghcr.io/latere-ai/origoweb:unreleased":       true,
		"ghcr.io/latere-ai/origoweb:v0.10.2":          true,
		"localhost:5000/origoweb:v0.10.2":             true,
		"ghcr.io/latere-ai/origoweb@sha256:0123abcd":  true,
		"ghcr.io/latere-ai/origoweb:latest@sha256:01": true,
	} {
		if got := imageTagProblem(ref) == ""; got != pinned {
			t.Errorf("imageTagProblem(%s) reports pinned=%t, want %t", ref, got, pinned)
		}
	}
}

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
// flow requests. An identity provider refuses an authorization request
// naming a scope the client does not hold, so a scope that outlives its
// grant is a sign-in that does not work rather than a feature that is
// absent. The interface needs no scope beyond the OpenID Connect minimum:
// the key store and the repository registry authorize the `api.latere.ai`
// actor token, not a login scope.
//
// The base is what every overlay inherits, and an overlay that names no
// scopes runs this set. An exact set also fails when the setting vanishes
// altogether, which would drop offline_access and kill the session at the
// first token expiry.
func TestTheInterfaceAsksOnlyForTheOIDCMinimum(t *testing.T) {
	env := containerEnv(t, filepath.Join(moduleRoot(t), "deploy/base/deployment.yaml"))

	entry, ok := env["ORIGOWEB_AUTH_SCOPES"]
	if !ok {
		t.Fatal("no manifest sets ORIGOWEB_AUTH_SCOPES")
	}
	if entry.fromRef {
		t.Fatal("ORIGOWEB_AUTH_SCOPES is declared from a reference, so the scope set that runs is not in the manifests and this test pins nothing")
	}
	got := map[string]bool{}
	for scope := range strings.SplitSeq(entry.value, ",") {
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

// TestASecretBackedSettingDoesNotFallBackToTheBase holds the reader the
// scope test rests on. kustomize keys env by name, so an overlay that
// declares ORIGOWEB_AUTH_SCOPES from a Secret replaces the base's literal
// outright: the value that runs is then in no manifest. A reader that
// returned literals only would report the base's string for a variable the
// base no longer supplies, and a test of the scope set would pass while
// pinning a set nothing applies. So the merged declaration must carry the
// reference, and the caller must refuse to assert on it.
func TestASecretBackedSettingDoesNotFallBackToTheBase(t *testing.T) {
	dir := t.TempDir()
	write := func(name, env string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		body := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: origoweb\n" +
			"spec:\n  template:\n    spec:\n      containers:\n        - name: origoweb\n" +
			"          env:\n" + env
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	base := write("base.yaml", "            - name: ORIGOWEB_AUTH_SCOPES\n              value: openid,email,profile,offline_access\n")
	// The overlay carries a literal beside the reference: a reader that
	// dropped references would still return a map for this file, and the
	// scope set would quietly stay the base's.
	overlay := write("overlay.yaml",
		"            - name: ORIGOWEB_PRODUCT_NAME\n              value: Latere Code\n"+
			"            - name: ORIGOWEB_AUTH_SCOPES\n              valueFrom:\n                secretKeyRef: {name: origoweb, key: auth-scopes}\n")

	env := map[string]envEntry{}
	for _, path := range []string{base, overlay} {
		maps.Copy(env, containerEnv(t, path))
	}

	entry, ok := env["ORIGOWEB_AUTH_SCOPES"]
	if !ok {
		t.Fatal("the merged environment lost ORIGOWEB_AUTH_SCOPES: the overlay declares it and the reader dropped the declaration")
	}
	if !entry.fromRef {
		t.Errorf("the overlay sources ORIGOWEB_AUTH_SCOPES from a Secret, and the merged declaration reports a literal %q: the base's value survived an overlay that replaced it", entry.value)
	}
	if entry.value != "" {
		t.Errorf("a valueFrom declaration carries the value %q, and the manifest names none", entry.value)
	}
}

// envEntry is one environment declaration on the container: a literal
// value, or a reference the cluster resolves at start-up, which names no
// value in the manifest.
type envEntry struct {
	value   string
	fromRef bool
}

// containerEnv reads the origoweb container's environment out of a
// Deployment or of a strategic-merge patch shaped like one. Every
// declaration is returned, a valueFrom included with fromRef set and no
// value: kustomize keys env by name, so an overlay that moved a setting to
// a Secret replaces the base's literal, and a helper that dropped the
// reference would leave a caller reading the base's value as though it were
// the one that runs.
func containerEnv(t *testing.T, path string) map[string]envEntry {
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
							Name      string         `yaml:"name"`
							Value     string         `yaml:"value"`
							ValueFrom map[string]any `yaml:"valueFrom"`
						} `yaml:"env"`
					} `yaml:"containers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	env := map[string]envEntry{}
	for _, container := range manifest.Spec.Template.Spec.Containers {
		if container.Name != "origoweb" {
			continue
		}
		for _, entry := range container.Env {
			env[entry.Name] = envEntry{value: entry.Value, fromRef: entry.ValueFrom != nil}
		}
	}
	if len(env) == 0 {
		t.Fatalf("%s sets no environment on the origoweb container", path)
	}
	return env
}
