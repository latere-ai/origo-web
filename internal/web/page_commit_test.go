// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/latere-ai/origo-web/internal/origo"
)

// TestACommitNamesItsCommitterWhenItIsNotTheAuthor asserts the second name on
// a commit.
//
// A rebase, a cherry-pick and an applied patch leave a commit whose author
// and committer are two people, and which of them did what is a fact about
// the commit. On the ordinary commit they are one person, and the screen
// says that person once.
func TestACommitNamesItsCommitterWhenItIsNotTheAuthor(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	const path = "/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"

	if body := text(elements(doc(t, h.get(h.repoPath(path), c).Body.String()), "body")[0]); strings.Contains(body, "Committer") {
		t.Error("a commit written and applied by one person names them twice")
	}

	at := time.Date(2026, 9, 10, 9, 41, 0, 0, time.UTC)
	h.fake.commits[0].Committer = origo.Person{Name: "m.okonkwo", Email: "mo@example.com", At: at.Add(time.Hour)}
	body := text(elements(doc(t, h.get(h.repoPath(path), c).Body.String()), "body")[0])
	if !strings.Contains(body, "Committer") || !strings.Contains(body, "m.okonkwo") {
		t.Errorf("a commit applied by a second person does not name them: %q", body)
	}
	if !strings.Contains(body, "a.hoshino") {
		t.Errorf("the author is gone from a commit that has a committer too: %q", body)
	}
}

// TestACommitOpensEveryFileOnAsking asserts the shut-file control.
//
// A file over the collapse budget renders shut with its body already in the
// document. Reading a whole commit then costs one click per file, so the
// screen offers one address that opens all of them, and one back. Neither
// appears where it would do nothing, and opening every file does not render
// a body the per-file rule dropped.
func TestACommitOpensEveryFileOnAsking(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	const path = "/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"

	// The fixture commit is four lines, so nothing is shut and there is
	// nothing to open.
	if link := expandLink(t, h.get(h.repoPath(path), c).Body.String()); link != "" {
		t.Errorf("a commit with nothing shut offers %q", link)
	}

	h.fake.diff = syntheticDiff(2000)
	link := expandLink(t, h.get(h.repoPath(path), c).Body.String())
	if link == "" {
		t.Fatal("a commit with a shut file offers no way to open it")
	}

	rec := h.get(link, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("the open link answered %d", rec.Code)
	}
	page := doc(t, rec.Body.String())
	for _, d := range classed(page, "diff-file") {
		if d.Data == "details" && !hasAttr(d, "open") {
			t.Error("a file is still shut on the screen that opens every file")
		}
	}
	if expandLink(t, rec.Body.String()) != "" {
		t.Error("the opened screen still offers to open every file")
	}
	if !strings.Contains(text(elements(page, "body")[0]), "Shut the large files") {
		t.Error("the opened screen offers no way back")
	}

	// A body the per-file rule dropped stays dropped: opening every file
	// asks a different question from rendering one that is too large.
	h.fake.diff = syntheticDiff(6000)
	page = doc(t, h.get(h.repoPath(path)+"?expand=1", c).Body.String())
	if !strings.Contains(text(elements(page, "body")[0]), "Too large to render inline") {
		t.Error("opening every file rendered a body the per-file budget dropped")
	}
}

// expandLink is the address that opens every shut file, empty when the screen
// offers none.
func expandLink(t *testing.T, body string) string {
	t.Helper()
	var out string
	find(doc(t, body), func(e *html.Node) {
		if e.Data == "a" && text(e) == "Open every file" {
			out = attr(e, "href")
		}
	})
	return out
}
