// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
