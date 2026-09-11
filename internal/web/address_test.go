// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestARepositoryIsAddressedByItsName asserts that every repository screen
// lives under /{owner}/{slug}, that the name is resolved through the name
// mode of Origo's collection route in one call, that no link the interface
// writes points at an identifier address, and that a name Origo does not
// know gets the one refusal.
func TestARepositoryIsAddressedByItsName(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	if !strings.HasPrefix(h.repoPath(""), "/infra/origo") {
		t.Fatalf("the fixture is addressed at %q", h.repoPath(""))
	}
	for name, path := range h.screens() {
		rec := h.get(path, c)
		if rec.Code != http.StatusOK {
			t.Errorf("%s at %s answers %d", name, path, rec.Code)
			continue
		}
		for _, a := range elements(doc(t, rec.Body.String()), "a") {
			if strings.HasPrefix(attr(a, "href"), "/r/") {
				t.Errorf("%s links to an identifier address %q", name, attr(a, "href"))
			}
		}
	}

	h.fake.Reset()
	h.get(h.repoPath("/refs"), c)
	if calls := h.fake.Calls(); len(calls) == 0 || calls[0] != "/v1/repos?owner&slug" {
		t.Errorf("the name was resolved with %v, want one call to the collection route", calls)
	}

	if rec := h.get("/infra/nothing", c); rec.Code != http.StatusNotFound {
		t.Errorf("a name Origo does not know answers %d, want the one refusal", rec.Code)
	}
}

// TestTheIdentifierAddressRedirectsToTheName asserts that /r/{id}, and
// every screen under it, answers with a permanent redirect to the name
// address, keeping the rest of the path and the query, so a link written
// before names were served still lands. An identifier the reader cannot
// open is the one refusal, so the redirect tells a stranger nothing. A
// clone address pasted into a browser lands on the repository too.
func TestTheIdentifierAddressRedirectsToTheName(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	for from, to := range map[string]string{
		h.idPath(""):                                                 h.repoPath(""),
		h.idPath("/refs"):                                            h.repoPath("/refs"),
		h.idPath("/log?ref=next"):                                    h.repoPath("/log?ref=next"),
		h.idPath("/tree/internal?ref=next"):                          h.repoPath("/tree/internal?ref=next"),
		h.idPath("/blob/README.md"):                                  h.repoPath("/blob/README.md"),
		h.idPath("/compare?base=main&head=next"):                     h.repoPath("/compare?base=main&head=next"),
		h.idPath("/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"): h.repoPath("/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"),
		h.idPath("/visibility"):                                      h.repoPath("/visibility"),
		h.idPath("/delete"):                                          h.repoPath("/delete"),
	} {
		rec := h.get(from, c)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != to {
			t.Errorf("%s answers %d to %q, want a permanent redirect to %s", from, rec.Code, rec.Header().Get("Location"), to)
		}
	}
	if rec := h.get(h.repoPath(".git"), c); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != h.repoPath("") {
		t.Errorf("a clone address answers %d to %q", rec.Code, rec.Header().Get("Location"))
	}

	h.fake.status["/v1/repos/1f2e3d"] = http.StatusForbidden
	if rec := h.get(h.idPath(""), c); rec.Code != http.StatusNotFound {
		t.Errorf("an identifier the reader cannot open answers %d, want the one refusal", rec.Code)
	}
}

// TestOpenTakesANameOrAnIdentifier asserts that the box on an installation
// with no directory takes what a person has: owner/name, or the identifier.
func TestOpenTakesANameOrAnIdentifier(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	for in, want := range map[string]string{
		"infra/origo": "/infra/origo",
		"1f2e3d":      "/r/1f2e3d",
	} {
		rec := h.get("/open?id="+in, c)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Errorf("opening %q answers %d to %q, want %s", in, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}
