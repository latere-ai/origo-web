// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The visibility screen: what it says before either change, who may see
// it, and that the overview carries the badge.

// TestVisibilityScreenSaysWhatChanges is the copy rule. A person reads one
// plain sentence per direction before they press the button, and the
// private direction says the thing a person has to be told: going private
// does not recall what was already cloned.
func TestVisibilityScreenSaysWhatChanges(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	c := h.signedIn("alice")

	h.registry.visibility = "private"
	body := h.get(h.repoPath("/visibility"), c).Body.String()
	for _, want := range []string{
		"This repository is private.",
		"Anyone can read and clone this repository. They do not need an account.",
		"The code, every branch, every tag, and the full history become readable.",
		"It does not appear in any list.",
		"Make public",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the private screen does not say %q", want)
		}
	}

	h.registry.visibility = "public"
	body = h.get(h.repoPath("/visibility"), c).Body.String()
	for _, want := range []string{
		"This repository is public.",
		"Only people you have given access can read this repository.",
		"Copies that were already cloned stay where they are. Making it private does not delete them.",
		"Make private",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the public screen does not say %q", want)
		}
	}
}

// TestVisibilityChangesBothWays is the control.
func TestVisibilityChangesBothWays(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	h.registry.visibility = "private"
	c := h.signedIn("alice")

	rec := h.post(h.repoPath("/visibility"), map[string][]string{"visibility": {"public"}}, c)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("making it public = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != h.repoPath("") {
		t.Errorf("Location = %q, want the repository", got)
	}

	rec = h.post(h.repoPath("/visibility"), map[string][]string{"visibility": {"private"}}, c)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("making it private = %d, want 303", rec.Code)
	}
	if got := h.registry.VisibilityWrites(); !slices.Equal(got, []string{"public", "private"}) {
		t.Errorf("the registry was written %v, want public then private", got)
	}
}

// TestVisibilityRefusesAValueItDoesNotKnow keeps the form honest: a
// submission naming neither visibility writes nothing.
func TestVisibilityRefusesAValueItDoesNotKnow(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	c := h.signedIn("alice")
	rec := h.post(h.repoPath("/visibility"), map[string][]string{"visibility": {"unlisted"}}, c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown visibility = %d, want 400", rec.Code)
	}
	if got := h.registry.VisibilityWrites(); len(got) != 0 {
		t.Errorf("the registry was written %v, want nothing", got)
	}
}

// TestVisibilityIsNotForAReader asserts that somebody who may read the
// repository but not administer it gets the interface's one refusal, the
// same answer an absent repository gets.
func TestVisibilityIsNotForAReader(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = false
	c := h.signedIn("alice")
	rec := h.get(h.repoPath("/visibility"), c)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a reader's visibility screen = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Make public") {
		t.Error("a reader was shown the control")
	}
}

// TestVisibilityIsAbsentWithoutARegistry covers the installation whose
// authorizer keeps no registry: the screen is a refusal and the overview
// still renders, because a fact that is not the overview's subject must
// not fail it.
func TestVisibilityIsAbsentWithoutARegistry(t *testing.T) {
	h := newHarness(t)
	h.registry.absent = true
	c := h.signedIn("alice")
	if rec := h.get(h.repoPath("/visibility"), c); rec.Code != http.StatusNotFound {
		t.Errorf("the visibility screen without a registry = %d, want 404", rec.Code)
	}
	rec := h.get(h.repoPath(""), c)
	if rec.Code != http.StatusOK {
		t.Fatalf("the overview without a registry = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "/visibility") {
		t.Error("the overview offered the control with no registry behind it")
	}
}

// TestOverviewShowsThePublicBadge is what a person reads at a glance.
func TestOverviewShowsThePublicBadge(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	c := h.signedIn("alice")

	h.registry.visibility = "private"
	body := h.get(h.repoPath(""), c).Body.String()
	if strings.Contains(body, ">public<") {
		t.Error("a private repository carries the public badge")
	}
	if !strings.Contains(body, h.repoPath("/visibility")) {
		t.Error("an administrator is not offered the control")
	}

	h.registry.visibility = "public"
	body = h.get(h.repoPath(""), c).Body.String()
	if !strings.Contains(body, ">public<") {
		t.Error("a public repository carries no badge")
	}
}

// TestVisibilitySendsThePersonsOwnToken keeps the delegation rule of the
// interface: it holds no credential and speaks only with the reader's.
func TestVisibilitySendsThePersonsOwnToken(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	c := h.signedIn("alice")
	h.get(h.repoPath("/visibility"), c)
	for _, tok := range h.registry.Tokens() {
		if tok == "" || !strings.HasPrefix(tok, "Bearer ") {
			t.Errorf("the registry was called with %q, want the reader's bearer", tok)
		}
	}
}
