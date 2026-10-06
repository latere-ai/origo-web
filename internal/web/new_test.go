// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/registry"
)

// The creation screen: who it offers, the one call it makes, where it lands,
// and what it says when the installation refuses.

// TestCreatingARepositoryLandsOnIt is the whole path a person walks. The
// registry writes the row and makes the repository, and the browser ends up
// on the repository rather than back on a form.
func TestCreatingARepositoryLandsOnIt(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")

	screen := h.get("/new", c)
	if screen.Code != http.StatusOK {
		t.Fatalf("the creation screen answers %d", screen.Code)
	}
	body := screen.Body.String()
	for _, want := range []string{"New repository", "alice", "infra", "Create repository"} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen does not offer %q", want)
		}
	}

	rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("creating answers %d, want 303: %s", rec.Code, rec.Body.String())
	}

	written := h.registry.Written()
	if len(written) != 1 || written[0].OwnerLabel != "alice" || written[0].Slug != "notes" {
		t.Fatalf("the registry holds %+v, want one row for alice/notes", written)
	}
	if made := h.fake.Created(); len(made) != 0 {
		t.Fatalf("the screen asked Origo to create %v; the registry makes the repository", made)
	}
	if got := rec.Header().Get("Location"); got != "/alice/notes" {
		t.Fatalf("lands on %q, want the repository at /alice/notes", got)
	}
	if len(h.registry.Forgotten()) != 0 {
		t.Fatalf("a successful creation withdrew %v", h.registry.Forgotten())
	}
}

// TestTheCreationScreenSendsOnlyTheReadersToken: this service holds no
// credential of its own, so a creation carries the person's authority and
// nothing else. The registry takes the token minted for the control plane's
// audience, and the session token, which is the issuer's, never leaves.
func TestTheCreationScreenSendsOnlyTheReadersToken(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	if rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c); rec.Code != http.StatusSeeOther {
		t.Fatalf("create: %d", rec.Code)
	}
	const session = "token-for-alice"
	var registryCalls int
	for i, tok := range h.registry.Tokens() {
		if strings.HasPrefix(h.registry.Calls()[i], "POST /actor-tokens") {
			continue
		}
		registryCalls++
		if tok != platformBearer(session) {
			t.Fatalf("a registry call carried %q, want the control plane's actor token %q",
				tok, platformBearer(session))
		}
		if tok == "Bearer "+session {
			t.Fatal("a registry call carried the session token")
		}
	}
	if registryCalls == 0 {
		t.Fatal("the registry was never called")
	}
}

// TestACreationIsOneCallToTheRegistry: the registry decides, writes its row
// and makes the repository at Origo as one operation, and answers once for
// both. The screen asks it once and asks Origo nothing, whether the answer
// is a repository or a refusal, and has nothing to take back afterwards.
func TestACreationIsOneCallToTheRegistry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
	}{
		{"a creation", 0, ""},
		{"a refused creation", http.StatusBadGateway, "origo_refused"},
		{"a creation the git host did not answer", http.StatusBadGateway, "origo_unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if tc.status != 0 {
				h.registry.refuseCreate(tc.status, tc.code)
			}
			c := h.signedIn("alice")
			h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c)

			var creates int
			for _, call := range h.registry.Calls() {
				switch {
				case call == "POST /repositories":
					creates++
				case strings.HasPrefix(call, "DELETE "):
					t.Errorf("the screen withdrew a row: %s", call)
				}
			}
			if creates != 1 {
				t.Errorf("the registry was asked to create %d times, want once", creates)
			}
			if made := h.fake.Created(); len(made) != 0 {
				t.Errorf("the screen asked Origo to create %v", made)
			}
		})
	}
}

// TestACreationLandsWhereTheRegistrySays: the registry answers with the
// owner name as it holds it, which can differ in case from what was typed,
// and the repository's address is the registry's.
func TestACreationLandsWhereTheRegistrySays(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	rec := h.post("/new", url.Values{"owner": {"ALICE"}, "name": {"notes"}}, c)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("creating answers %d, want 303: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/alice/notes" {
		t.Fatalf("lands on %q, want the address the registry answered, /alice/notes", got)
	}
}

// TestEveryRefusalIsItsOwnSentence: the registry says which rule stopped a
// creation, and the screen says it in the words of the person it happened
// to, with the form filled in as they left it.
func TestEveryRefusalIsItsOwnSentence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		want   int
		says   string
	}{
		{"a name that is not yours", http.StatusForbidden, "forbidden", http.StatusForbidden, "cannot create repositories under that owner"},
		{"a name already taken", http.StatusConflict, "conflict", http.StatusConflict, "already exists under that owner"},
		{"an owner at its limit", http.StatusConflict, registry.CodeAtTheLimit, http.StatusConflict, "reached its repository limit"},
		{"a request the registry will not read", http.StatusBadRequest, "invalid_request", http.StatusBadRequest, "Choose an owner and a name"},
		{"a repository the installation refused", http.StatusBadGateway, "origo_refused", http.StatusConflict, "was not created. The installation refused it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.registry.refuseCreate(tc.status, tc.code)
			c := h.signedIn("alice")
			rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c)
			if rec.Code != tc.want {
				t.Fatalf("answers %d, want %d", rec.Code, tc.want)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.says) {
				t.Errorf("the screen does not say %q: %s", tc.says, body)
			}
			// Nothing is retyped: the name comes back on the form.
			if !strings.Contains(body, `value="notes"`) {
				t.Error("a refused submission lost the name that was typed")
			}
			if len(h.fake.Created()) != 0 {
				t.Error("a refused creation reached the installation")
			}
		})
	}
}

// TestACreationTheGitHostDidNotFinishIsUnavailable: the registry did not
// get an answer from Origo, or cannot reach it at all on this installation.
// Nothing was created, and the screen is the one every unanswered call gets.
func TestACreationTheGitHostDidNotFinishIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{http.StatusBadGateway, "origo_unreachable"},
		{http.StatusServiceUnavailable, "origo_unavailable"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			h := newHarness(t)
			h.registry.refuseCreate(tc.status, tc.code)
			c := h.signedIn("alice")
			rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c)
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("answers %d, want 502", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), unavailableSentence) {
				t.Errorf("the screen does not say the server is not responding: %s", rec.Body.String())
			}
		})
	}
}

// TestANameTheInstallationWillNotTakeIsRefusedBeforeARowIsWritten: the shape
// of a name is checked here, so a name Origo would reject never becomes a
// registry row.
func TestANameTheInstallationWillNotTakeIsRefusedBeforeARowIsWritten(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	for _, name := range []string{"", "-leading", "a/b", "a b", strings.Repeat("x", 65)} {
		rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {name}}, c)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name %q answers %d, want 400", name, rec.Code)
		}
	}
	if rec := h.post("/new", url.Values{"owner": {""}, "name": {"notes"}}, c); rec.Code != http.StatusBadRequest {
		t.Errorf("no owner answers %d, want 400", rec.Code)
	}
	if got := h.registry.Written(); len(got) != 0 {
		t.Fatalf("a name the installation would refuse became %d rows", len(got))
	}
}

// TestAPersonWithNoNameIsToldWhatToDo: a repository lives under a name, so a
// person who holds none gets the one thing they can do next rather than a
// form that could not have worked.
func TestAPersonWithNoNameIsToldWhatToDo(t *testing.T) {
	h := newHarness(t)
	h.registry.setNamespaces(registry.Namespaces{})
	c := h.signedIn("alice")

	rec := h.get("/new", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("answers %d, want 200: this is a document, not a refusal", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Create repository") {
		t.Error("a form is offered to somebody who has no name to create under")
	}
	for _, want := range []string{"No owner available", "Go to your account"} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen does not say %q: %s", want, body)
		}
	}
}

// TestTheCreationAffordanceFollowsTheInstallation: the repositories screen
// offers a way in only when a signed-in person is looking and only when this
// installation has a component that records who owns a repository.
func TestTheCreationAffordanceFollowsTheInstallation(t *testing.T) {
	h := newHarness(t)
	if got := h.get("/", h.signedIn("alice")).Body.String(); !strings.Contains(got, `href="/new"`) {
		t.Error("a signed-in person is offered no way to create a repository")
	}

	// An installation with no such component offers nothing, and the
	// address answers that it is not served here.
	off := newHarness(t, func(c *config.Config) { c.OIDC.AuthURL = "" })
	c := off.signedIn("alice")
	if got := off.get("/", c).Body.String(); strings.Contains(got, `href="/new"`) {
		t.Error("an installation that registers nothing still offers a way to create")
	}
	if rec := off.get("/new", c); rec.Code != http.StatusNotFound {
		t.Errorf("the creation address answers %d on such an installation, want 404", rec.Code)
	}
}

// TestTheCreationScreenNeedsASession: a signed-out visitor gets the front
// door, and a submission with no form token is refused before anything is
// written.
func TestTheCreationScreenNeedsASession(t *testing.T) {
	h := newHarness(t)
	if rec := h.get("/new"); !strings.Contains(rec.Body.String(), "Sign in") {
		t.Errorf("a signed-out visitor does not get the front door: %s", rec.Body.String())
	}
	req, _ := http.NewRequest(http.MethodPost, "/new", strings.NewReader("owner=alice&name=notes"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(h.signedIn("alice"))
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a submission with no form token answers %d, want 403", rec.Code)
	}
	if len(h.registry.Written()) != 0 || len(h.fake.Created()) != 0 {
		t.Error("a submission with no form token created something")
	}
}
