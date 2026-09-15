// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/latere-ai/origo-web/internal/config"
)

// TestEachServiceReceivesItsOwnActorToken pins which credential each call
// carries. Origo takes a token the issuer minted for Origo; the registry
// and the key store, which are the control plane's now, take a token minted
// for the control plane's audience. The session token is addressed to the
// issuer and goes to neither.
func TestEachServiceReceivesItsOwnActorToken(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.KeysURL = mustURL(t, "https://keys.example") })
	c := h.signedIn("alice")
	const session = "token-for-alice"
	actor := "Bearer " + actorTokenFor("origo", session)

	if rec := h.get(h.repoPath(""), c); rec.Code != http.StatusOK {
		t.Fatalf("repository page = %d", rec.Code)
	}
	if rec := h.get("/new", c); rec.Code != http.StatusOK {
		t.Fatalf("creation page = %d", rec.Code)
	}
	if rec := h.get("/keys", c); rec.Code != http.StatusOK {
		t.Fatalf("keys page = %d", rec.Code)
	}

	h.fake.mu.Lock()
	origoTokens := append([]string(nil), h.fake.tokens...)
	h.fake.mu.Unlock()
	if len(origoTokens) == 0 {
		t.Fatal("Origo was not called")
	}
	for _, got := range origoTokens {
		if got != actor {
			t.Errorf("Origo received %q, want the actor token %q", got, actor)
		}
	}

	h.registry.mu.Lock()
	minted := append([]string(nil), h.registry.minted...)
	registryTokens := append([]string(nil), h.registry.tokens...)
	registryCalls := append([]string(nil), h.registry.calls...)
	h.registry.mu.Unlock()
	// One mint per audience on this session: the issuer is asked once for
	// Origo and once for the control plane, and the library caches each.
	wantMinted := []string{"origo for " + session, "api.latere.ai for " + session}
	if !slices.Equal(minted, wantMinted) {
		t.Errorf("the issuer minted %v, want %v", minted, wantMinted)
	}
	var registryCalled bool
	for i, call := range registryCalls {
		if strings.HasPrefix(call, "POST /actor-tokens") {
			continue
		}
		registryCalled = true
		if registryTokens[i] != platformBearer(session) {
			t.Errorf("the registry received %q on %s, want the control plane's actor token %q",
				registryTokens[i], call, platformBearer(session))
		}
		if registryTokens[i] == "Bearer "+session {
			t.Errorf("the registry received the session token on %s", call)
		}
	}
	if !registryCalled {
		t.Error("the registry was never called")
	}

	h.keys.mu.Lock()
	keyTokens := append([]string(nil), h.keys.bearer...)
	h.keys.mu.Unlock()
	if len(keyTokens) == 0 {
		t.Fatal("the key store was not called")
	}
	want := actorTokenFor("api.latere.ai", session)
	for _, got := range keyTokens {
		if got != want {
			t.Errorf("the key store received %q, want the control plane's actor token %q", got, want)
		}
		if got == session {
			t.Error("the key store received the session token")
		}
	}
}

// TestIssuerFaultIsSaidBehindTheSession pins what a signed-in person sees
// when the issuer mints neither token: not a stranger's page, and not a
// sign-out, but the sentence that says what happened.
func TestIssuerFaultIsSaidBehindTheSession(t *testing.T) {
	h := newHarness(t)
	h.registry.mu.Lock()
	h.registry.refuseMint = true
	h.registry.mu.Unlock()
	c := h.signedIn("alice")

	rec := h.get("/", c)
	if !strings.Contains(rec.Body.String(), issuerFaultSentence) {
		t.Errorf("the home page does not say the issuer failed:\n%s", rec.Body.String())
	}
	if rec.Header().Get("Set-Cookie") != "" && strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Error("an issuer fault must not sign the person out")
	}
	// The address the person asked for carries its own code, which wins.
	rec = h.get("/?auth_error=access_denied", c)
	if body := rec.Body.String(); !strings.Contains(body, authRefusedSentence) || strings.Contains(body, issuerFaultSentence) {
		t.Errorf("the code on the address must win over the fault:\n%s", body)
	}
}

// TestThePlatformMintFailsAlone is the state between the two releases: the
// issuer has not granted this client the control plane's audience yet, so it
// refuses that mint and mints for Origo as usual.
//
// The screens that read Origo must be unaffected, and the two that talk to
// the control plane must say what happened rather than read as a stranger's
// page with nothing on it.
func TestThePlatformMintFailsAlone(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.KeysURL = mustURL(t, "https://keys.example") })
	h.registry.mu.Lock()
	h.registry.refuseAudience = "api.latere.ai"
	h.registry.mu.Unlock()
	c := h.signedIn("alice")

	// The home screen reads Origo, which was minted for, so it says nothing
	// about the identity provider.
	home := h.get("/", c)
	if home.Code != http.StatusOK {
		t.Fatalf("the home page = %d", home.Code)
	}
	if strings.Contains(home.Body.String(), issuerFaultSentence) {
		t.Error("a screen reading Origo says the issuer failed; only the control plane's mint did")
	}

	// The two that need the control plane say the sentence instead of
	// offering a form that cannot work.
	for _, path := range []string{"/new", "/keys"} {
		rec := h.get(path, c)
		if !strings.Contains(rec.Body.String(), issuerFaultSentence) {
			t.Errorf("%s does not say the issuer did not mint:\n%s", path, rec.Body.String())
		}
	}

	// And no registry call fell back to the session token. The mint route
	// is the issuer's own and is the one place that token belongs.
	calls := h.registry.Calls()
	for i, tok := range h.registry.Tokens() {
		if strings.HasPrefix(calls[i], "POST /actor-tokens") {
			continue
		}
		t.Errorf("the registry was called (%s with %q) without a token for it", calls[i], tok)
	}
	if got := h.keys.bearers(); len(got) != 0 {
		t.Errorf("the key store was called with %v, want no call at all", got)
	}
}
