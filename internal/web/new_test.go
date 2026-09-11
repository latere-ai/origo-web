// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"latere.ai/x/pkg/authkit"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/registry"
)

// The creation screen: who it offers, what it writes, where it lands, and
// what it leaves behind when the second half fails.

// TestCreatingARepositoryLandsOnIt is the whole path a person walks. The row
// is written first and the repository second, and the browser ends up on the
// repository rather than back on a form.
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
	made := h.fake.Created()
	if len(made) != 1 || made[0].Owner != "alice" || made[0].Slug != "notes" {
		t.Fatalf("the installation made %+v, want one repository alice/notes", made)
	}
	if made[0].ID != written[0].ID {
		t.Fatalf("the row names %q and the repository %q; one creation is one id",
			written[0].ID, made[0].ID)
	}
	if got := rec.Header().Get("Location"); got != "/alice/notes" {
		t.Fatalf("lands on %q, want the repository at /alice/notes", got)
	}
	if len(h.registry.Forgotten()) != 0 {
		t.Fatalf("a successful creation withdrew %v", h.registry.Forgotten())
	}
}

// TestTheCreationScreenSendsOnlyTheReadersToken: this service holds no
// credential of its own, so both halves of a creation carry the person's
// own token and nothing else.
func TestTheCreationScreenSendsOnlyTheReadersToken(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	if rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c); rec.Code != http.StatusSeeOther {
		t.Fatalf("create: %d", rec.Code)
	}
	tokens := h.registry.Tokens()
	if len(tokens) == 0 {
		t.Fatal("the registry was never called")
	}
	for _, tok := range tokens {
		if !strings.HasPrefix(tok, "Bearer ") {
			t.Fatalf("a registry call carried %q, want the reader's bearer", tok)
		}
	}
	// And the same token reached Origo, which is what makes the two halves
	// one person's authority rather than this service's.
	if got := h.fake.Tokens(); len(got) == 0 || got[len(got)-1] != tokens[0] {
		t.Fatalf("Origo saw %v and the registry saw %v; they must be one credential", got, tokens)
	}
}

// TestACreationRetriesOnlyTheRefusalTheRegistryLagProduces.
//
// A registry row reaches the replica that served the write first, and
// Origo's authorize call lands on any of them, so a fresh row can be
// answered unknown_repository once. That one refusal is retried. Every
// other is the answer.
func TestACreationRetriesOnlyTheRefusalTheRegistryLagProduces(t *testing.T) {
	h := newHarness(t)
	h.fake.unknownFor = 1
	shorten(t)
	c := h.signedIn("alice")

	if rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c); rec.Code != http.StatusSeeOther {
		t.Fatalf("a creation that raced the registry answers %d, want 303", rec.Code)
	}
	if made := h.fake.Created(); len(made) != 1 {
		t.Fatalf("the installation made %d repositories, want one", len(made))
	}
	if len(h.registry.Forgotten()) != 0 {
		t.Fatal("a creation that succeeded on the second attempt withdrew its row")
	}

	// A refusal that is not the lag is not retried, and the row goes.
	other := newHarness(t)
	other.fake.status["/v1/repos"] = http.StatusForbidden
	oc := other.signedIn("alice")
	rec := other.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, oc)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a refused creation answers %d, want 409", rec.Code)
	}
	if len(other.fake.Created()) != 0 {
		t.Fatal("a refused creation made a repository")
	}
	if got := other.registry.Forgotten(); len(got) != 1 {
		t.Fatalf("a refused creation left %v behind; the row must go with it", got)
	}
}

// TestACreationThatFailsKeepsNothing: the person closed the tab rather than
// retrying, so a row that outlived its repository would hold the name
// against its own owner forever.
func TestACreationThatFailsKeepsNothing(t *testing.T) {
	h := newHarness(t)
	h.fake.unknownFor = createAttempts + 1
	shorten(t)
	c := h.signedIn("alice")

	rec := h.post("/new", url.Values{"owner": {"alice"}, "name": {"notes"}}, c)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a creation the authorizer never allowed answers %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "was not created") {
		t.Errorf("the screen does not say the name is free again: %s", rec.Body.String())
	}
	written := h.registry.Written()
	if len(written) != 1 {
		t.Fatalf("the registry saw %d writes, want one", len(written))
	}
	if got := h.registry.Forgotten(); len(got) != 1 || got[0] != written[0].ID {
		t.Fatalf("withdrew %v, want the one row %q that was written", got, written[0].ID)
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

// shorten makes the retry's wait short enough to drive, and puts it back.
// The wait is longer than Origo's deny cache in the running service, which
// is a property of the service and not of this suite.
func shorten(t *testing.T) {
	t.Helper()
	was := createBackoff
	createBackoff = time.Millisecond
	t.Cleanup(func() { createBackoff = was })
}

// TestTheRowIsWithdrawnEvenWhenTheBrowserIsGone.
//
// The compensating withdrawal exists for the person who submits the form and
// closes the tab. That cancels the request while the creation is still
// retrying, so a withdrawal made on the request's own context would fail at
// once and leave behind exactly the row it was added to remove.
func TestTheRowIsWithdrawnEvenWhenTheBrowserIsGone(t *testing.T) {
	h := newHarness(t)
	h.fake.unknownFor = createAttempts + 1
	shorten(t)
	c := h.signedIn("alice")

	// The form token first, on a request that completes.
	token := h.csrf("/new", c)
	form := url.Values{"owner": {"alice"}, "name": {"notes"}}
	form.Set(authkit.CSRFFieldName(), token)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/new", strings.NewReader(form.Encode())).WithContext(ctx)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(c)
	for _, ck := range h.csrfCookies {
		req.AddCookie(ck)
	}

	// The browser goes while the creation is between attempts.
	go func() {
		for range 200 {
			if len(h.registry.Written()) > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	h.server.ServeHTTP(httptest.NewRecorder(), req)

	written := h.registry.Written()
	if len(written) != 1 {
		t.Fatalf("the registry saw %d writes, want one", len(written))
	}
	if got := h.registry.Forgotten(); len(got) != 1 || got[0] != written[0].ID {
		t.Fatalf("withdrew %v after the browser went, want the one row %q", got, written[0].ID)
	}
}
