// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"latere.ai/x/pkg/authkit"
)

// linksTo reports whether the page carries a link to the address.
func linksTo(page *html.Node, href string) bool {
	for _, a := range elements(page, "a") {
		if attr(a, "href") == href {
			return true
		}
	}
	return false
}

// recentCookie is the recent-repository cookie a response set, nil when it
// set none.
func recentCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "__Host-origoweb-recent" {
			return ck
		}
	}
	return nil
}

// postWith is h.post with more cookies on the request, for a form whose
// effect on another cookie is the thing under test.
func (h *harness) postWith(path string, form url.Values, c *http.Cookie, more ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	form.Set(authkit.CSRFFieldName(), h.csrf(path, c))
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(c)
	for _, ck := range h.csrfCookies {
		req.AddCookie(ck)
	}
	for _, ck := range more {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	return rec
}

// TestDeletingARepositoryAsksFirst asserts that an administrator is offered
// the deletion from the overview, that the screen names the repository and
// says what happens before the button, that it asks for the name to be
// typed back, and that looking at it deletes nothing.
func TestDeletingARepositoryAsksFirst(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	c := h.signedIn("alice")

	if !linksTo(doc(t, h.get(h.repoPath(""), c).Body.String()), h.repoPath("/delete")) {
		t.Error("the overview offers an administrator no way to delete the repository")
	}

	rec := h.get(h.repoPath("/delete"), c)
	if rec.Code != http.StatusOK {
		t.Fatalf("the deletion screen answers %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Delete infra/origo",
		"Nobody can read, clone, push to or open it",
		"Existing clones are not affected.",
		"seven days",
		"restore it",
		"Delete repository",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen does not say %q", want)
		}
	}
	var typed bool
	for _, in := range elements(doc(t, body), "input") {
		if attr(in, "name") == "name" && attr(in, "type") != "hidden" {
			typed = true
		}
	}
	if !typed {
		t.Error("the screen does not ask for the name to be typed")
	}
	if len(h.fake.Deleted()) != 0 || len(h.registry.Forgotten()) != 0 {
		t.Error("looking at the screen deleted something")
	}
}

// TestDeletingARepositoryNeedsItsNameTyped asserts the safeguard and the
// deletion itself: a name that does not match deletes nothing and says so;
// the right one deletes the repository at Origo, then withdraws its
// ownership row, drops it from what this session opened, and lands on the
// list, which says what happened.
func TestDeletingARepositoryNeedsItsNameTyped(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	c := h.signedIn("alice")

	rec := h.post(h.repoPath("/delete"), url.Values{"name": {"infra/origin"}}, c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a wrong name answers %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "exactly as shown") {
		t.Errorf("a wrong name is not told what to do:\n%s", rec.Body.String())
	}
	if len(h.fake.Deleted()) != 0 || len(h.registry.Forgotten()) != 0 {
		t.Fatal("a wrong name deleted something")
	}

	recent := recentCookie(h.get(h.repoPath(""), c))
	if recent == nil {
		t.Fatal("opening the repository did not remember it")
	}
	rec = h.postWith(h.repoPath("/delete"), url.Values{"name": {"infra/origo"}}, c, recent)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("deleting answers %d, want 303: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/?deleted=infra%2Forigo" {
		t.Errorf("deleting lands on %q", got)
	}
	if got := h.fake.Deleted(); len(got) != 1 || got[0] != "1f2e3d" {
		t.Errorf("Origo was asked to delete %v", got)
	}
	if got := h.registry.Forgotten(); len(got) != 1 || got[0] != "1f2e3d" {
		t.Errorf("the registry was asked to forget %v", got)
	}
	var recents []*http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "__Host-origoweb-recent" {
			recents = append(recents, ck)
		}
	}
	if len(recents) != 1 || (recents[0].Value != "" && recents[0].MaxAge >= 0) {
		t.Errorf("the deleted repository is still in what this session opened: %+v", recents)
	}

	home := h.get("/?deleted=infra%2Forigo", c).Body.String()
	if !strings.Contains(home, "infra/origo was deleted") {
		t.Errorf("the list does not say what was deleted:\n%s", home)
	}
	// The address is not repeated unless it is shaped like a name.
	if junk := h.get("/?deleted=%3Cb%3Ehello%3C%2Fb%3E", c).Body.String(); strings.Contains(junk, "was deleted") {
		t.Error("the list repeats whatever the address says")
	}
}

// TestDeletionIsNotForAReader asserts that a person the registry does not
// name as an administrator is not offered the deletion, and gets the one
// refusal from both the screen and the form, so nobody learns which of
// absent and withheld it was.
func TestDeletionIsNotForAReader(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")

	if linksTo(doc(t, h.get(h.repoPath(""), c).Body.String()), h.repoPath("/delete")) {
		t.Error("the overview offers a reader the deletion")
	}
	if rec := h.get(h.repoPath("/delete"), c); rec.Code != http.StatusNotFound {
		t.Errorf("the screen answers a reader %d, want 404", rec.Code)
	}

	// A form token from any page is a valid one, so the refusal below is
	// the registry's answer and not the token check.
	form := url.Values{"name": {"infra/origo"}}
	form.Set(authkit.CSRFFieldName(), h.csrf(h.repoPath(""), c))
	req := httptest.NewRequest(http.MethodPost, h.repoPath("/delete"), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(c)
	for _, ck := range h.csrfCookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("the form answers a reader %d, want 404", rec.Code)
	}
	if len(h.fake.Deleted()) != 0 || len(h.registry.Forgotten()) != 0 {
		t.Error("a reader deleted something")
	}
}

// TestADeletionOrigoRefusesWithdrawsNoRow asserts the order: Origo first,
// the ownership row second. A deletion Origo refuses leaves the row, so
// the repository stands under its name instead of standing under none.
func TestADeletionOrigoRefusesWithdrawsNoRow(t *testing.T) {
	h := newHarness(t)
	h.registry.canChange = true
	h.fake.deleteStatus = http.StatusForbidden
	c := h.signedIn("alice")

	rec := h.post(h.repoPath("/delete"), url.Values{"name": {"infra/origo"}}, c)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a refused deletion answers %d, want the one refusal", rec.Code)
	}
	if got := h.registry.Forgotten(); len(got) != 0 {
		t.Errorf("the row was withdrawn after Origo refused: %v", got)
	}
}
