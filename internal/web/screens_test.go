// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"latere.ai/x/pkg/authkit/oidc"

	"github.com/latere-ai/origo-web/internal/config"
	"github.com/latere-ai/origo-web/internal/keys"
	"github.com/latere-ai/origo-web/internal/origo"
)

// screens is every screen a signed-in person can reach on the fixture
// repository, named as the criteria name them.
func (h *harness) screens() map[string]string {
	return map[string]string{
		"home":     "/",
		"overview": h.repoPath(""),
		"refs":     h.repoPath("/refs"),
		"log":      h.repoPath("/log"),
		"commit":   h.repoPath("/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"),
		"compare":  h.repoPath("/compare?base=main&head=next"),
		"tree":     h.repoPath("/tree/internal"),
		"file":     h.repoPath("/blob/README.md"),
		"tokens":   "/tokens",
		"new":      "/new",
		"docs":     "/docs/agents",
	}
}

// everyPage renders every screen a page-wide property must hold on: the ten
// a signed-in reader reaches, and the front door a signed-out one does. The
// signed-out page is the one a stranger sees, so no property may skip it.
func (h *harness) everyPage() map[string]*httptest.ResponseRecorder {
	h.t.Helper()
	c := h.signedIn("alice")
	out := make(map[string]*httptest.ResponseRecorder, len(h.screens())+5)
	for name, path := range h.screens() {
		out[name] = h.get(path, c)
	}
	out["sign in"] = h.get("/sign-in")
	maps.Copy(out, keyScreens(h.t, h.cfg))
	return out
}

// keyScreens renders the key screen in each of the three states it has.
//
// It needs a harness of its own because the screen exists only where the
// installation runs a key store, and the screens above are the ones every
// installation has. The three states are separate pages to a reader even
// though they are one address: the table and the add form, the parsed key
// waiting for a yes, and the removal asking before it acts. A page-wide
// property that held on the first and not the other two would be a property
// that did not hold.
//
// base is the calling harness's own configuration, carried over so the key
// screens are the same installation as the screens beside them: a test that
// names the installation and draws its mark must see them here too.
func keyScreens(t *testing.T, base config.Config) map[string]*httptest.ResponseRecorder {
	t.Helper()
	withKeys := func(cfg *config.Config) {
		*cfg = base
		cfg.KeysURL = mustURL(t, "https://keys.example")
	}
	h := newHarness(t, withKeys)
	used := time.Now().Add(-6 * 24 * time.Hour)
	added := time.Now().Add(-200 * 24 * time.Hour)
	h.keys.add(keys.Key{
		ID: "k1", Comment: "aki@thinkpad", Type: "ssh-ed25519", Bits: 256,
		Fingerprint: "SHA256:9Xk2pL0rTqYb1nH4vDcE7mZaW8sJfQ6uR3gN5oB2iVc",
		Created:     &added, LastUsed: &used,
	})
	h.keys.add(keys.Key{
		ID: "k2", Comment: "", Type: "ssh-rsa", Bits: 4096,
		Fingerprint: "SHA256:Pw4Lm8Rt2Yq6Ze1Ns0Vb7Ud3Xj9Kc5Ga2Hf8Oi4Tr0",
		Created:     &added,
	})
	c := h.signedIn("alice")
	out := map[string]*httptest.ResponseRecorder{
		"keys":        h.get("/keys", c),
		"key removal": h.get("/keys/k1/remove", c),
	}
	out["key confirm"] = h.post("/keys", url.Values{"key": {"ssh-ed25519 AAAAnewkey aki@framework"}}, c)

	// An empty account is a page a person meets on their first visit, so
	// the properties hold on it too.
	empty := newHarness(t, withKeys)
	out["keys empty"] = empty.get("/keys", empty.signedIn("alice"))
	return out
}

// TestScreenCallsAreExact holds each screen to the calls spec 023's screen
// table lists for it, against a recording installation.
//
// The table addresses a repository by owner and slug, through a route Origo
// does not serve yet; every screen here resolves the repository by its id
// instead, with GET /v1/repos/{id} of spec 003, which is the same one call in
// the same place. No screen's call count grows with the number of rows it
// renders, which the second half of this test measures by rendering a wider
// page and counting again.
func TestScreenCallsAreExact(t *testing.T) {
	repo := "/v1/repos/1f2e3d"
	want := map[string][]string{
		"overview": {
			repo,
			repo + "/refs?prefix", repo + "/refs?prefix",
			repo + "/tree/main",
			repo + "/commits?limit&ref",
			repo + "/blob/b1",
		},
		"refs": {repo, repo + "/refs?prefix", repo + "/refs?prefix"},
		"log":  {repo, repo + "/commits?limit&ref"},
		"commit": {
			repo,
			repo + "/commits/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d",
			repo + "/compare/4c02f7e10b4d3a1e8f6b2c9d05a7e3f1b8c4d6e2...9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d",
		},
		"compare": {repo, repo + "/compare/main...next"},
		"tree":    {repo, repo + "/tree/main?path"},
		"file":    {repo, repo + "/tree/main", repo + "/blob/b1"},
		// The token screen asks the one question the home screen asks,
		// and asks nothing about a repository until one is named.
		"tokens": {"/v1/repos?limit"},
		// The documentation page is written from configuration alone.
		"docs": nil,
	}

	h := newHarness(t)
	c := h.signedIn("alice")
	for name, path := range h.screens() {
		if name == "home" {
			continue
		}
		h.fake.Reset()
		if rec := h.get(path, c); rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", name, rec.Code)
		}
		got := h.fake.Calls()
		if strings.Join(got, "\n") != strings.Join(want[name], "\n") {
			t.Errorf("%s made\n  %v\nwant\n  %v", name, got, want[name])
		}
	}

	// The same screens over a repository with many more rows make the same
	// number of calls: no screen asks a question per row.
	h.fake.trees["internal"] = manyEntries(500)
	h.fake.commits = manyCommits(200)
	for name, path := range h.screens() {
		if name == "home" || name == "overview" || name == "commit" || name == "docs" {
			continue
		}
		h.fake.Reset()
		h.get(path, c)
		if got, wanted := len(h.fake.Calls()), len(want[name]); got != wanted {
			t.Errorf("%s over a wider page made %d calls, want %d", name, got, wanted)
		}
	}
}

// TestRefusalAndAbsenceAreIndistinguishable asserts that a 403 and a 404 from
// Origo reach the reader as one sentence and one status. Origo refuses to say
// which of the two it meant, and this interface refuses to leak the
// difference.
func TestRefusalAndAbsenceAreIndistinguishable(t *testing.T) {
	var forbidden, missing *httptest.ResponseRecorder
	for i, code := range []int{http.StatusForbidden, http.StatusNotFound} {
		h := newHarness(t)
		h.fake.status["/v1/repos/1f2e3d"] = code
		rec := h.get(h.repoPath(""), h.signedIn("alice"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("origo answered %d, the page answered %d, want 404", code, rec.Code)
		}
		if i == 0 {
			forbidden = rec
		} else {
			missing = rec
		}
	}
	// The two pages are the same but for the token each render issues to
	// its own form, which is per response by construction.
	if withoutCSRF(forbidden.Body.String()) != withoutCSRF(missing.Body.String()) {
		t.Error("a refusal and an absence rendered different pages")
	}
	if !strings.Contains(forbidden.Body.String(), "No such repository, or you cannot see it") {
		t.Errorf("the sentence is not the one the spec fixes:\n%s", forbidden.Body.String())
	}
}

// TestAnonymousRendersWhateverOrigoAnswers asserts both halves of the rule:
// today a signed-out visitor is shown the sign-in page, because Origo refuses
// a tokenless read; and the same handler, against an installation that
// answers one, renders the repository, with no branch on "is there a session"
// between them.
func TestAnonymousRendersWhateverOrigoAnswers(t *testing.T) {
	// Half one: the installation refuses a request with no credential.
	h := newHarness(t)
	h.fake.status["/v1/repos/1f2e3d"] = http.StatusUnauthorized
	for _, path := range []string{h.repoPath(""), h.repoPath("/log"), h.repoPath("/tree/")} {
		rec := h.get(path)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401 with the sign-in page", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "/auth/start") {
			t.Errorf("%s: the page offers no way to sign in", path)
		}
	}

	// Half two: the same handler, the same request, an installation that
	// answers a read carrying no Authorization header.
	h2 := newHarness(t)
	rec := h2.get(h2.repoPath(""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want the repository", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "origo") {
		t.Error("the repository did not render for a reader with no session")
	}
	for _, got := range h2.fake.Tokens() {
		if got != "" {
			t.Errorf("a tokenless read carried %q; it must carry no Authorization header at all", got)
		}
	}
}

// TestEveryScreenWorksWithoutScript asserts the property the design and the
// spec both fix: no screen contains a script, and every navigation on it is a
// link to a real URL or a form that submits.
func TestEveryScreenWorksWithoutScript(t *testing.T) {
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())

		if got := elements(page, "script"); len(got) > 0 {
			t.Errorf("%s carries %d script elements", name, len(got))
		}
		for _, e := range elements(page, "a") {
			href := attr(e, "href")
			if href == "" {
				t.Errorf("%s: a link with no href: %q", name, text(e))
			}
			if strings.HasPrefix(strings.ToLower(href), "javascript:") {
				t.Errorf("%s: a link that runs a script: %q", name, href)
			}
		}
		for _, e := range elements(page, "form") {
			if attr(e, "action") == "" {
				t.Errorf("%s: a form with no action", name)
			}
			method := strings.ToLower(attr(e, "method"))
			if method != "get" && method != "post" {
				t.Errorf("%s: a form with method %q", name, method)
			}
		}
		for _, e := range elements(page, "button") {
			if attr(e, "type") == "button" {
				t.Errorf("%s: a button that only a script could act on", name)
			}
		}
		for _, k := range []string{"onclick", "onchange", "onsubmit", "onload"} {
			if strings.Contains(rec.Body.String(), k+"=") {
				t.Errorf("%s: an inline %s handler", name, k)
			}
		}
	}
}

// TestNoCacheIsSharedBetweenSubjects asserts the rule the spec writes down so
// nobody optimises it away: a rendered page is never served to a second
// subject. This service keeps no cache, and every page says so, so the
// question cannot arise in a cache in front of it either.
func TestNoCacheIsSharedBetweenSubjects(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.signedIn("alice"), h.signedIn("bob")

	first := h.get(h.repoPath(""), alice)
	if got := first.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control is %q, want private, no-store", got)
	}

	// The installation now refuses the second reader. If any page had been
	// kept, the refusal would be answered from it.
	h.fake.status["/v1/repos/1f2e3d"] = http.StatusForbidden
	second := h.get(h.repoPath(""), bob)
	if second.Code != http.StatusNotFound {
		t.Fatalf("the second reader got %d, want the refusal", second.Code)
	}
	if strings.Contains(second.Body.String(), "Clone") {
		t.Error("the second reader was served the first reader's page")
	}

	// Each reader's own token went out with their own request.
	tokens := h.fake.Tokens()
	if len(tokens) < 2 {
		t.Fatalf("want a call per reader, got %d", len(tokens))
	}
	if tokens[0] == tokens[len(tokens)-1] {
		t.Error("two readers' calls carried the same credential")
	}
}

// TestStaleNotice asserts that Origo-Stale reaches the page as a sentence,
// and that the content is otherwise the same. A machine consumer ignores that
// header; a person reading a commit log must not.
func TestStaleNotice(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	plain := h.get(h.repoPath("/log"), c).Body.String()
	if strings.Contains(plain, "may be a few minutes behind") {
		t.Fatal("a consistent response carried the staleness line")
	}

	h.fake.headers["/v1/repos/1f2e3d/commits"] = http.Header{"Origo-Stale": []string{"180"}}
	stale := h.get(h.repoPath("/log"), c).Body.String()
	if !strings.Contains(stale, "may be a few minutes behind") {
		t.Error("a stale response carried no staleness line")
	}
	if !strings.Contains(stale, "3 minutes") {
		t.Errorf("the line does not say how far behind:\n%s", stale)
	}
	if !strings.Contains(stale, "web: collapse diffs over the render budget") {
		t.Error("a stale response lost its content")
	}
}

// TestRoutesAreReadOnly asserts the route table against a checked-in list.
//
// This interface has no write path to repository content of any kind. The
// five routes that are not a GET are sign-out, the mint form, the creation
// form and the two on the key screen, each of which requires a token issued
// to this session, and none of which edits a file, moves a reference, or
// changes a repository that exists. Creating brings an empty repository
// into being, which is a different thing from writing to one. A key is an
// account's credential: adding one writes to the installation's key store
// and touches no repository at all.
func TestRoutesAreReadOnly(t *testing.T) {
	want := []string{
		"GET /{$}", "GET /sign-in", "GET /open", "GET /auth/start", "GET /auth/callback",
		"POST /sign-out", "GET /assets/{file}",
		"GET /tokens", "POST /tokens", "GET /new", "POST /new", "GET /docs/agents",
		"GET /r/{id}", "GET /r/{id}/refs", "GET /r/{id}/log", "GET /r/{id}/commit/{sha}",
		"GET /r/{id}/patch/{sha}", "GET /r/{id}/compare", "GET /r/{id}/tree/{path...}",
		"GET /r/{id}/blob/{path...}", "GET /r/{id}/raw/{path...}",
		"GET /keys", "POST /keys", "GET /keys/{id}/remove", "POST /keys/{id}/remove",
	}
	var got []string
	for _, r := range Routes(true) {
		got = append(got, r.Method+" "+r.Pattern)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the route table is\n%v\nand the checked-in list is\n%v", got, want)
	}
	for _, r := range Routes(true) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			t.Errorf("%s %s: a method that is neither a read nor one of the two forms", r.Method, r.Pattern)
		}
	}

	// Every form refuses a submission with no token.
	h := newHarness(t, func(c *config.Config) { c.KeysURL = mustURL(t, "https://keys.example") })
	for _, path := range []string{"/sign-out", "/keys", "/tokens", "/new"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.AddCookie(h.signedIn("alice"))
		rec := httptest.NewRecorder()
		h.server.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST %s with no token: %d, want 403", path, rec.Code)
		}
	}

	// The list above is what the server registered, and the server is what
	// answers: every address of the interface is driven with every method
	// that changes something, and the only three that answer are the three
	// forms. A handler added straight onto the mux would fail here.
	c := h.signedIn("alice")
	addresses := []string{
		"/", "/sign-in", "/open", "/auth/start", "/auth/callback", "/sign-out",
		"/keys", "/tokens", "/new", "/docs/agents", "/assets/app.css", h.repoPath(""), h.repoPath("/refs"),
		h.repoPath("/log"), h.repoPath("/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"),
		h.repoPath("/patch/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"), h.repoPath("/compare"),
		h.repoPath("/tree/internal"), h.repoPath("/blob/README.md"), h.repoPath("/raw/README.md"),
		"/livez", "/readyz", "/version",
	}
	for _, address := range addresses {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if method == http.MethodPost && (address == "/sign-out" || address == "/keys" ||
				address == "/tokens" || address == "/new") {
				continue
			}
			req := httptest.NewRequest(method, address, nil)
			req.AddCookie(c)
			rec := httptest.NewRecorder()
			h.server.ServeHTTP(rec, req)
			if rec.Code < 400 {
				t.Errorf("%s %s answered %d; nothing but the two forms may change anything",
					method, address, rec.Code)
			}
		}
	}
}

// TestKeyScreenIsOptional asserts that the key screen is absent by default,
// together with its navigation entry, and that nothing else changes when it
// appears.
//
// Origo holds no public key: it asks a store the operator runs whose an
// offered key is. An operator who runs no such store has no place to add
// one, so this interface offers none rather than offering a screen that
// cannot work.
func TestKeyScreenIsOptional(t *testing.T) {
	off := newHarness(t)
	c := off.signedIn("alice")
	if rec := off.get("/keys", c); rec.Code != http.StatusNotFound {
		t.Errorf("the key screen answers %d while no key store is configured, want 404", rec.Code)
	}
	if rec := off.get("/keys/k1/remove", c); rec.Code != http.StatusNotFound {
		t.Errorf("a removal answers %d with no key store, want 404", rec.Code)
	}
	home := off.get("/", c).Body.String()
	if strings.Contains(home, "ssh keys") {
		t.Error("the navigation offers a screen that is not there")
	}

	on := newHarness(t, func(cfg *config.Config) { cfg.KeysURL = mustURL(t, "https://keys.example") })
	c2 := on.signedIn("alice")
	if rec := on.get("/keys", c2); rec.Code != http.StatusOK {
		t.Errorf("the configured key screen answers %d", rec.Code)
	}
	if !strings.Contains(on.get("/", c2).Body.String(), "ssh keys") {
		t.Error("the configured key screen has no navigation entry")
	}

	// Every other screen renders the same either way.
	for name, path := range off.screens() {
		a := off.get(path, c).Code
		b := on.get(path, c2).Code
		if a != b {
			t.Errorf("%s answers %d without the key screen and %d with it", name, a, b)
		}
	}
}

// TestCloneURLsComeFromConfiguration asserts that the clone addresses come
// from the settings an operator gave the process and never from a request
// header, so a forged Host cannot move them.
func TestCloneURLsComeFromConfiguration(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.SSHCloneHost = "git.example" })
	req := httptest.NewRequest(http.MethodGet, h.repoPath(""), nil)
	req.Host = "attacker.example"
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.AddCookie(h.signedIn("alice"))
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "https://git.example/infra/origo.git") {
		t.Errorf("the HTTPS clone address is not the configured one:\n%s", body)
	}
	if strings.Contains(body, "attacker.example") {
		t.Error("a request header reached the page")
	}

	// The screen shows one address at a time and the other is a link to
	// this same screen, so the SSH form is a second render and not a
	// second string hidden in the first one.
	ssh := h.get(h.repoPath("")+"?clone=ssh", h.signedIn("alice")).Body.String()
	if !strings.Contains(ssh, "git@git.example:infra/origo.git") {
		t.Errorf("the SSH clone address is not the configured one:\n%s", ssh)
	}
	if strings.Contains(ssh, "attacker.example") {
		t.Error("a request header reached the page")
	}

	// With no SSH surface configured, only the HTTPS form is shown.
	plain := newHarness(t)
	body = plain.get(plain.repoPath(""), plain.signedIn("alice")).Body.String()
	if strings.Contains(body, "git@") {
		t.Error("an installation with no SSH surface showed an SSH clone address")
	}
}

// TestReferenceSelector asserts that the control lists the branches and the
// tags the installation returned, and that choosing one is a form submission
// that lands on the same screen at the new revision.
func TestReferenceSelector(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	page := doc(t, h.get(h.repoPath(""), c).Body.String())

	var options []string
	var form *html.Node
	for _, in := range elements(page, "input") {
		if attr(in, "name") != "ref" {
			continue
		}
		if attr(in, "type") != "radio" {
			t.Errorf("the reference control is a %q", attr(in, "type"))
		}
		options = append(options, attr(in, "value"))
		for p := in.Parent; p != nil; p = p.Parent {
			if p.Data == "form" {
				form = p
				break
			}
		}
	}
	want := []string{"main", "next", "v1.4.2"}
	if strings.Join(options, ",") != strings.Join(want, ",") {
		t.Errorf("the control offers %v, the installation returned %v", options, want)
	}
	if form == nil {
		t.Fatal("the control is not inside a form")
	}
	if strings.ToLower(attr(form, "method")) != "get" {
		t.Errorf("the control submits with %q", attr(form, "method"))
	}

	// Submitting it lands on the same screen at the new revision.
	rec := h.get(attr(form, "action")+"?ref=next", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("the chosen reference answered %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Files at next") {
		t.Error("the screen did not move to the chosen reference")
	}
}

// TestListDegradesWithoutDirectory asserts that a home screen without a
// collection route is the way in that works without one, and that the rest of
// the interface is unaffected.
func TestListDegradesWithoutDirectory(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")

	body := h.get("/", c).Body.String()
	if !strings.Contains(body, "does not list repositories") {
		t.Errorf("the home screen does not say it cannot enumerate:\n%s", body)
	}
	page := doc(t, body)
	forms := elements(page, "form")
	var open bool
	for _, f := range forms {
		if attr(f, "action") == "/open" {
			open = true
		}
	}
	if !open {
		t.Error("the home screen offers no way to open a repository")
	}

	// Opening one leaves it in the list for the rest of the session.
	rec := h.get(h.repoPath(""), c)
	var recent *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "__Host-origoweb-recent" {
			recent = ck
		}
	}
	if recent == nil {
		t.Fatal("opening a repository did not remember it")
	}
	if !strings.Contains(h.get("/", c, recent).Body.String(), "Opened in this session") {
		t.Error("the home screen does not show what this session opened")
	}

	// Every other screen is unaffected.
	for name, path := range h.screens() {
		if name == "home" {
			continue
		}
		if rec := h.get(path, c); rec.Code != http.StatusOK {
			t.Errorf("%s answers %d on an installation with no directory", name, rec.Code)
		}
	}
}

// TestTokenNeverLeavesTheCookie asserts that the access token appears in no
// body, no URL, and no header of any screen.
func TestTokenNeverLeavesTheCookie(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	const token = "token-for-alice"
	for name, path := range h.screens() {
		rec := h.get(path, c)
		if strings.Contains(rec.Body.String(), token) {
			t.Errorf("%s: the token is in the body", name)
		}
		for k, vs := range rec.Header() {
			for _, v := range vs {
				if strings.Contains(v, token) && k != "Set-Cookie" {
					t.Errorf("%s: the token is in the %s header", name, k)
				}
			}
		}
		page := doc(t, rec.Body.String())
		for _, a := range elements(page, "a") {
			if strings.Contains(attr(a, "href"), token) {
				t.Errorf("%s: the token is in a URL", name)
			}
		}
	}
	// The cookie itself is encrypted: the token is not readable in it.
	if strings.Contains(c.Value, token) {
		t.Error("the session cookie carries the token in the clear")
	}
}

// manyEntries and manyCommits widen a page so a call count can be measured
// against the number of rows it renders.
func manyEntries(n int) []origo.Entry {
	out := make([]origo.Entry, 0, n)
	for i := range n {
		out = append(out, origo.Entry{
			Path: "internal/f" + itoa(i) + ".go", Mode: "100644", Type: "blob",
			SHA: "b" + itoa(i), Size: int64(100 + i),
		})
	}
	return out
}

func manyCommits(n int) []origo.Commit {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	out := make([]origo.Commit, 0, n)
	for i := range n {
		out = append(out, origo.Commit{
			SHA:     strings.Repeat("a", 39) + itoa(i%10),
			Parents: []string{strings.Repeat("b", 40)},
			Author:  origo.Person{Name: "a.hoshino", Email: "aki@example.com", At: at},
			Message: "a commit " + itoa(i),
		})
	}
	return out
}

// withoutCSRF removes the per-response form token, which differs between any
// two renders and says nothing about what a page shows.
func withoutCSRF(body string) string {
	const marker = `value="`
	i := strings.Index(body, "csrf_token")
	if i < 0 {
		return body
	}
	j := strings.Index(body[i:], marker)
	if j < 0 {
		return body
	}
	start := i + j + len(marker)
	k := strings.Index(body[start:], `"`)
	if k < 0 {
		return body
	}
	return body[:start] + body[start+k:]
}

// TestTheInterfaceNamesTheProductItIs asserts the rule that keeps one
// operator's branding out of another's installation: every place the
// interface names itself reads the name from configuration, and an
// installation nobody named is the open-source project under the project's
// own name.
func TestTheInterfaceNamesTheProductItIs(t *testing.T) {
	// The default. Nothing here was configured, so nothing here is
	// anybody's product.
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		body := rec.Body.String()
		page := doc(t, body)
		title := text(elements(page, "title")[0])
		if !strings.HasSuffix(title, "· "+config.ProjectName) {
			t.Errorf("%s: the title is %q", name, title)
		}
		if got := text(brand(t, page)); got != config.ProjectName {
			t.Errorf("%s: the masthead says %q", name, got)
		}
		if strings.Contains(body, "Latere Code") {
			t.Errorf("%s: an installation nobody named carries somebody's product name", name)
		}
	}

	// And an operator who named theirs is named on every screen, in the
	// masthead and in the tab.
	h = newHarness(t, func(c *config.Config) { c.ProductName = "Latere Code" })
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		if got := text(brand(t, page)); got != "Latere Code" {
			t.Errorf("%s: the masthead says %q", name, got)
		}
		if title := text(elements(page, "title")[0]); !strings.HasSuffix(title, "· Latere Code") {
			t.Errorf("%s: the title is %q", name, title)
		}
	}
}

// brand is the masthead's own link, the one that names the installation.
func brand(t *testing.T, page *html.Node) *html.Node {
	t.Helper()
	for _, a := range elements(page, "a") {
		if attr(a, "class") == "brand" {
			return a
		}
	}
	t.Fatal("the page has no masthead brand")
	return nil
}

// TestTheMarkBelongsToTheInstallationNotTheSoftware asserts the other half of
// the branding rule. A logo is its owner's: the software draws none unless an
// operator asked for one by name, and the one this build carries is Latere's.
//
// The mark is inline, in currentColor, so it is ink in both themes, fetches
// nothing, and is sized in its own attributes so it is right before the
// stylesheet arrives and if it never does.
func TestTheMarkBelongsToTheInstallationNotTheSoftware(t *testing.T) {
	plain := newHarness(t)
	for name, rec := range plain.everyPage() {
		if got := elements(doc(t, rec.Body.String()), "svg"); len(got) > 0 {
			t.Errorf("%s: an installation that asked for no mark was drawn %d", name, len(got))
		}
	}

	h := newHarness(t, func(c *config.Config) {
		c.ProductName = "Latere Code"
		c.Mark = config.MarkLatere
	})
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		marks := elements(page, "svg")
		if len(marks) == 0 {
			t.Errorf("%s: the installation asked for a mark and got none", name)
			continue
		}
		if len(elements(brand(t, page), "svg")) != 1 {
			t.Errorf("%s: the mark is drawn somewhere other than the masthead", name)
		}
		for _, svg := range marks {
			if attr(svg, "fill") != "currentColor" {
				t.Errorf("%s: the mark is filled with %q, so it is one colour in two themes", name, attr(svg, "fill"))
			}
			if attr(svg, "width") == "" || attr(svg, "height") == "" {
				t.Errorf("%s: the mark has no size of its own, so it has none without the stylesheet", name)
			}
			if attr(svg, "style") != "" {
				t.Errorf("%s: the mark carries an inline style, which the policy refuses", name)
			}
			if len(elements(svg, "path")) != 6 {
				t.Errorf("%s: the mark drew %d paths", name, len(elements(svg, "path")))
			}
			// Beside the product name the mark is decorative: the
			// name is the accessible name, and a second one would
			// be read twice.
			if attr(svg, "aria-hidden") != "true" {
				t.Errorf("%s: the mark beside the name is not marked decorative", name)
			}
		}
	}
}

// TestTheIdentityAppearsOnceOnAScreen asserts the rule that keeps the
// interface from introducing itself twice in one view: the masthead is where
// this installation says which one it is, because it is on every page, and
// nothing below it repeats the name or the mark. The screen a stranger lands
// on leads with what they can do here instead.
func TestTheIdentityAppearsOnceOnAScreen(t *testing.T) {
	named := newHarness(t, func(c *config.Config) {
		c.ProductName = "Latere Code"
		c.Mark = config.MarkLatere
	})
	for name, rec := range named.everyPage() {
		page := doc(t, rec.Body.String())
		body := textOutside(elements(page, "body")[0], "readme")
		if got := strings.Count(body, "Latere Code"); got != 1 {
			t.Errorf("%s: the screen names the installation %d times, want once", name, got)
		}
		if got := text(brand(t, page)); got != "Latere Code" {
			t.Errorf("%s: the one naming is not the masthead's, which says %q", name, got)
		}
		if got := len(elements(page, "svg")); got != 1 {
			t.Errorf("%s: the mark is drawn %d times, want once, in the masthead", name, got)
		}
	}

	// An installation nobody named and nobody gave a mark: the masthead
	// carries the project's own name, alone, and no screen restates it as a
	// heading of its own.
	plain := newHarness(t)
	for name, rec := range plain.everyPage() {
		page := doc(t, rec.Body.String())
		if got := len(elements(page, "svg")); got != 0 {
			t.Errorf("%s: an installation that asked for no mark was drawn %d", name, got)
		}
		if got := text(brand(t, page)); got != config.ProjectName {
			t.Errorf("%s: the masthead says %q", name, got)
		}
		for _, tag := range []string{"h1", "h2", "h3"} {
			for _, h := range elements(page, tag) {
				if text(h) == config.ProjectName && !within(h, "readme") {
					t.Errorf("%s: a %s repeats the name the masthead carries", name, tag)
				}
			}
		}
	}

	// And the page a stranger lands on leads with what they can do, which
	// reads the same whether or not the installation has a name.
	for _, h := range []*harness{named, plain} {
		gate := doc(t, h.get("/sign-in").Body.String())
		if got := text(elements(gate, "h1")[0]); got != "Sign in to read git repositories" {
			t.Errorf("the signed-out page leads with %q", got)
		}
	}
}

// TestTheSignedInAccountIsNamed asserts what a person needs before they can
// read an empty page: which account they are using. What a reader may see is
// exactly what their credential may fetch, so the account is the first thing
// that explains nothing being there.
//
// The masthead carries it on every screen, once, beside the way out. The page
// a signed-out visitor sees carries no account at all, because there is none.
func TestTheSignedInAccountIsNamed(t *testing.T) {
	h := newHarness(t)
	c := h.signedInAs(oidc.User{Sub: "01HQ8Z", Email: "aki@example.com"})
	for name, path := range h.screens() {
		page := doc(t, h.get(path, c).Body.String())
		labels := accountLabels(page)
		if len(labels) != 1 {
			t.Errorf("%s names the account %d times, want once", name, len(labels))
			continue
		}
		if got := text(labels[0]); got != "aki@example.com" {
			t.Errorf("%s: the masthead says %q", name, got)
		}
		if !within(labels[0], "masthead") {
			t.Errorf("%s: the account is named outside the masthead", name)
		}
		// Beside the way out, so the two controls that are about the
		// session sit together.
		forms := elements(elements(page, "header")[0], "form")
		if len(forms) != 1 || attr(forms[0], "action") != "/sign-out" {
			t.Errorf("%s: the masthead has no sign-out form beside the account", name)
		}
		// The subject is not a name, and is not shown while a claim
		// that reads as one is there.
		if strings.Contains(rendered(page), "01HQ8Z") {
			t.Errorf("%s: the account identifier is shown while an address is known", name)
		}
	}

	// A signed-out visitor is nobody, and the front door says so by
	// carrying no account and offering the way in instead.
	gate := doc(t, h.get("/sign-in").Body.String())
	if got := len(accountLabels(gate)); got != 0 {
		t.Errorf("the signed-out page names an account %d times", got)
	}
	if strings.Contains(rendered(gate), "aki@example.com") {
		t.Error("the signed-out page carries an address")
	}
}

// TestTheAccountShownIsTheMostHumanClaimTheTokenCarries asserts the fallback
// through a rendered page, so the chain the session package chooses is the
// chain a reader sees.
func TestTheAccountShownIsTheMostHumanClaimTheTokenCarries(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		name string
		user oidc.User
		want string
	}{
		{"a name and an address", oidc.User{Sub: "01HQ8Z", Email: "aki@example.com", Name: "Aki Hoshino"}, "Aki Hoshino"},
		{"an address alone", oidc.User{Sub: "01HQ8Z", Email: "aki@example.com"}, "aki@example.com"},
		{"a subject alone", oidc.User{Sub: "01HQ8Z"}, "01HQ8Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := doc(t, h.get("/", h.signedInAs(tc.user)).Body.String())
			labels := accountLabels(page)
			if len(labels) != 1 {
				t.Fatalf("the account is named %d times", len(labels))
			}
			if got := text(labels[0]); got != tc.want {
				t.Errorf("the masthead says %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAPageWithNothingOnItSaysWhichAccountItIsEmptyFor asserts the second
// place the account belongs. A reader on a screen with no repositories on it
// is looking at the answer their credential produced, and the screen says so
// rather than leaving them to guess whether they are signed in at all.
func TestAPageWithNothingOnItSaysWhichAccountItIsEmptyFor(t *testing.T) {
	h := newHarness(t)
	c := h.signedInAs(oidc.User{Sub: "01HQ8Z", Email: "aki@example.com"})

	// The installation that serves no directory at all, which is every
	// installation today.
	body := textOutside(elements(doc(t, h.get("/", c).Body.String()), "body")[0], "readme")
	if !strings.Contains(body, "Signed in as aki@example.com") {
		t.Errorf("the screen without a directory does not say whose it is: %q", body)
	}

	// And the one that serves an empty one.
	h.answering(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"repos": []origo.Repo{}, "next_cursor": ""})
	})
	empty := textOutside(elements(doc(t, h.get("/", c).Body.String()), "body")[0], "readme")
	if !strings.Contains(empty, "Signed in as aki@example.com") {
		t.Errorf("the empty directory does not say whose it is: %q", empty)
	}

	// A signed-out visitor is told nothing about an account.
	if got := h.get("/sign-in").Body.String(); strings.Contains(strings.ToLower(got), "signed in as") {
		t.Error("the signed-out page claims somebody is signed in")
	}
}

// accountLabels are the masthead's account labels on a page, which must be
// one when somebody is signed in and none when nobody is.
func accountLabels(page *html.Node) []*html.Node {
	var out []*html.Node
	find(page, func(e *html.Node) {
		if attr(e, "class") == "who" {
			out = append(out, e)
		}
	})
	return out
}

// rendered is the page's text, the readme included, which is where a value
// that must appear nowhere is looked for.
func rendered(page *html.Node) string { return text(elements(page, "body")[0]) }

// helpRoles are the roles that explain something to a reader in passing: the
// sentence a screen opens with, the line under a field, the line on a screen
// with nothing on it, and what one choice means. They are the text that goes
// mannered first, because each one is short enough to be written for effect.
var helpRoles = []string{"lead", "hint", "empty", "option-note"}

// TestAHelpLineIsShort holds the interface's help text to a ceiling.
//
// Style is not a thing a Go test can read, and this does not try to. It
// measures the two things that go with prose written to sound considered:
// length, and sentences that keep going. The line this replaced ran to 182
// characters and a 30-word sentence to say "sign in with your organisation
// account, there is no separate password".
//
// The ceilings are above what every line on every screen runs to today, so
// this fails when a line grows rather than the moment anyone edits one.
func TestAHelpLineIsShort(t *testing.T) {
	const (
		maxChars = 160
		maxWords = 18
	)
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		find(page, func(e *html.Node) {
			classes := strings.Fields(attr(e, "class"))
			if !slices.ContainsFunc(helpRoles, func(r string) bool { return slices.Contains(classes, r) }) {
				return
			}
			line := strings.Join(strings.Fields(text(e)), " ")
			if len(line) > maxChars {
				t.Errorf("%s: a help line is %d characters, want %d or fewer: %q", name, len(line), maxChars, line)
			}
			for sentence := range strings.SplitSeq(line, ". ") {
				if n := len(strings.Fields(sentence)); n > maxWords {
					t.Errorf("%s: a sentence in a help line is %d words, want %d or fewer: %q", name, n, maxWords, sentence)
				}
			}
		})
	}
}

// TestNoScreenArguesWithItself bans the one mannered construction that is
// mechanically visible: "X, not Y". It is a rhetorical shape rather than an
// instruction, and a reader who wants to know what to do has to work out
// which half is the instruction. Two plain sentences say the same thing.
func TestNoScreenArguesWithItself(t *testing.T) {
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		body := text(elements(doc(t, rec.Body.String()), "body")[0])
		if i := strings.Index(body, ", not "); i >= 0 {
			t.Errorf("%s: a sentence is built as \"X, not Y\" rather than saying what to do: %q",
				name, body[max(0, i-60):min(len(body), i+60)])
		}
	}
}

// TestNoScreenCarriesAnEmDash asserts the punctuation rule of every text
// surface this repository owns: no em dash, anywhere a reader can meet one.
//
// A dash stands in for a decision the sentence should have made. A colon, a
// full stop, or two sentences say the same thing and read plainly, and a pair
// of things joined by a dash is usually two elements the stylesheet should be
// laying out. The three surfaces here are the templates, the sentences this
// package holds in Go, and the release notes.
func TestNoScreenCarriesAnEmDash(t *testing.T) {
	const emDash = "—"

	// Every screen, as a reader receives it.
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		if strings.Contains(rec.Body.String(), emDash) {
			t.Errorf("%s carries an em dash", name)
		}
	}

	// And the sources, so a sentence no fixture reaches is held to the
	// same rule: the templates, this package's Go strings, and the notes
	// every release is read from.
	roots := []string{"templates", ".", "../../CHANGELOG.md", "../../README.md"}
	for _, root := range roots {
		for _, path := range filesUnder(t, root) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(body), "\n") {
				if !strings.Contains(line, emDash) {
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue // a comment is written for a developer
				}
				t.Errorf("%s:%d carries an em dash: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// filesUnder is the text this repository owns at one place: a directory of
// templates, the Go of this package, or one named file.
func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		return []string{root}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !strings.HasSuffix(name, ".gohtml") && !strings.HasSuffix(name, ".go") {
			continue
		}
		out = append(out, filepath.Join(root, name))
	}
	return out
}
