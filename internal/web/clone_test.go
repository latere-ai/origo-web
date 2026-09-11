// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"

	"github.com/latere-ai/origo-web/internal/config"
)

// TestTheCloneAddressSaysWhatGitWillAskFor asserts that the clone box says
// what git asks for at each address: a token as the password over HTTPS,
// and a registered key over SSH. A person who pasted the HTTPS address after
// adding a key was asked for a username, with nothing on the screen to say
// why or what to type.
func TestTheCloneAddressSaysWhatGitWillAskFor(t *testing.T) {
	h := newHarness(t, func(c *config.Config) {
		c.SSHCloneHost = "git.example"
		c.KeysURL = mustURL(t, "https://keys.example")
	})
	c := h.signedIn("alice")

	https := firstWithClass(doc(t, h.get(h.repoPath(""), c).Body.String()), "clone")
	if https == nil {
		t.Fatal("the overview has no clone box")
	}
	if got := text(https); !strings.Contains(got, "token") {
		t.Errorf("the HTTPS address does not say a token is the password: %q", got)
	}
	var toTokens bool
	for _, a := range elements(https, "a") {
		if attr(a, "href") == "/tokens?repo=1f2e3d" {
			toTokens = true
		}
	}
	if !toTokens {
		t.Error("the HTTPS address does not lead to the token screen for this repository")
	}

	ssh := firstWithClass(doc(t, h.get(h.repoPath("")+"?clone=ssh", c).Body.String()), "clone")
	if got := text(ssh); !strings.Contains(got, "key") {
		t.Errorf("the SSH address does not say a key is what it uses: %q", got)
	}
	var toKeys bool
	for _, a := range elements(ssh, "a") {
		if attr(a, "href") == "/keys" {
			toKeys = true
		}
	}
	if !toKeys {
		t.Error("the SSH address does not lead to the key screen")
	}
}
