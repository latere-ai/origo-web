// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/latere-ai/origo-web/internal/config"
)

// TestOrigoReceivesAnActorTokenAndAuthTheSessionToken pins which credential
// each call carries. Origo takes a token the issuer minted for Origo and
// for this person; the issuer's own registry and key store take the session
// token, which is addressed to the issuer and opens nothing at Origo.
func TestOrigoReceivesAnActorTokenAndAuthTheSessionToken(t *testing.T) {
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
	if len(minted) != 1 || minted[0] != "origo for "+session {
		t.Errorf("the issuer minted %v, want one token for origo on the session", minted)
	}
	for i, call := range registryCalls {
		if strings.HasPrefix(call, "POST /actor-tokens") {
			continue
		}
		if registryTokens[i] != "Bearer "+session {
			t.Errorf("the registry received %q on %s, want the session token", registryTokens[i], call)
		}
	}

	h.keys.mu.Lock()
	keyTokens := append([]string(nil), h.keys.bearer...)
	h.keys.mu.Unlock()
	if len(keyTokens) == 0 {
		t.Fatal("the key store was not called")
	}
	for _, got := range keyTokens {
		if got != session {
			t.Errorf("the key store received %q, want the session token", got)
		}
	}
}

// TestIssuerFaultIsSaidBehindTheSession pins what a signed-in person sees
// when the issuer does not mint for Origo: not a stranger's page, and not a
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
