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
		"Anyone can read and clone it without an account.",
		"All branches, tags and history become readable.",
		"The repository is not listed publicly.",
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
		"Only people with access can read it.",
		"Existing clones are not affected.",
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
//
// Each state gets a harness of its own, because the answer is cached per
// reader and per repository: changing the registry under a live session
// is a change made elsewhere, and the overview is entitled to its cached
// answer until visibilityTTL runs out.
func TestOverviewShowsThePublicBadge(t *testing.T) {
	overview := func(t *testing.T, current string) (string, *harness) {
		t.Helper()
		h := newHarness(t)
		h.registry.canChange = true
		h.registry.visibility = current
		return h.get(h.repoPath(""), h.signedIn("alice")).Body.String(), h
	}

	body, h := overview(t, "private")
	if strings.Contains(body, ">public<") {
		t.Error("a private repository carries the public badge")
	}
	if !strings.Contains(body, ">private<") {
		t.Error("a private repository does not say so")
	}
	if !strings.Contains(body, h.repoPath("/visibility")) {
		t.Error("an administrator is not offered the control")
	}

	body, _ = overview(t, "public")
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

// TestAFailedVisibilityCallDoesNotReadAsPrivate is the defect this three
// state answer exists for.
//
// If a failed registry call rendered as private, a public repository would
// render exactly like a private one. That is a wrong answer rather than a
// missing one, and it is wrong in the direction nobody reports: the screen
// looks fine to the reader, and the person who made the repository public
// is not told that it stopped saying so.
//
// So the test does not assert that the badge is absent. It renders the
// same repository three ways and asserts the failure is distinguishable
// from both answers.
func TestAFailedVisibilityCallDoesNotReadAsPrivate(t *testing.T) {
	render := func(t *testing.T, set func(*fakeRegistry)) string {
		t.Helper()
		h := newHarness(t)
		h.registry.canChange = true
		set(h.registry)
		rec := h.get(h.repoPath(""), h.signedIn("alice"))
		if rec.Code != http.StatusOK {
			t.Fatalf("the overview = %d, want 200 whatever the registry did", rec.Code)
		}
		return rec.Body.String()
	}

	public := render(t, func(f *fakeRegistry) { f.visibility = "public" })
	private := render(t, func(f *fakeRegistry) { f.visibility = "private" })
	// The registry is reachable and refuses to answer. Not a 404, which
	// is an installation with no visibility surface and has nothing to
	// report, but a failure that should have produced an answer.
	broken := render(t, func(f *fakeRegistry) { f.visibilityStatus = http.StatusBadGateway })

	if broken == private {
		t.Fatal("a failed registry call renders exactly like a private repository; a public one would read as private and nobody would be told")
	}
	if broken == public {
		t.Fatal("a failed registry call renders exactly like a public repository")
	}
	if !strings.Contains(broken, "unavailable") {
		t.Errorf("the failure does not say so in words: the reader sees an absence and concludes private")
	}
	// And the two real answers stay distinguishable from each other.
	if public == private {
		t.Fatal("a public repository renders exactly like a private one")
	}
	if !strings.Contains(public, ">public<") || !strings.Contains(private, ">private<") {
		t.Error("the two answers are not each named on the screen")
	}
}

// TestVisibilityIsAskedOncePerReader pins the call the overview makes. The
// answer is cached per reader and per repository, so a second render of
// the same screen asks nothing, and a flip is written through so the
// person who made it sees it at once rather than after the lifetime.
func TestVisibilityIsAskedOncePerReader(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	h.registry.visibility = "private"
	c := h.signedIn("alice")

	asks := func() int {
		n := 0
		for _, call := range h.registry.Calls() {
			if strings.HasPrefix(call, "GET ") && strings.HasSuffix(call, "/visibility") {
				n++
			}
		}
		return n
	}

	h.get(h.repoPath(""), c)
	first := asks()
	if first != 1 {
		t.Fatalf("the first overview asked %d times, want once", first)
	}
	for range 4 {
		h.get(h.repoPath(""), c)
	}
	if got := asks(); got != first {
		t.Errorf("five renders asked %d times, want the one", got)
	}

	// A flip is written through, so the overview it redirects to shows
	// what was chosen without asking again and without waiting out the
	// lifetime.
	if rec := h.post(h.repoPath("/visibility"), map[string][]string{"visibility": {"public"}}, c); rec.Code != http.StatusSeeOther {
		t.Fatalf("the flip = %d, want 303", rec.Code)
	}
	body := h.get(h.repoPath(""), c).Body.String()
	if !strings.Contains(body, ">public<") {
		t.Error("the overview after a flip does not show what was just chosen")
	}
}
