// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestTheTreeOfAnEmptyRepositorySaysSo asserts that the tree tab of a
// repository nobody has pushed to is the same screen as every other tab,
// with a sentence in place of a listing.
//
// Origo answers the tree of such a repository with a missing reference, the
// same answer it gives for a branch that was never pushed. The interface
// used to render that as "this repository does not exist" under no sections
// at all, so the one tab the overview offered led to a dead end.
func TestTheTreeOfAnEmptyRepositorySaysSo(t *testing.T) {
	h := newHarness(t)
	h.fake.repo.Head = ""
	h.fake.repo.PushedAt = nil
	h.fake.heads, h.fake.tags, h.fake.commits = nil, nil, nil
	h.fake.status["/v1/repos/1f2e3d/tree/main"] = http.StatusNotFound

	for _, path := range []string{"/tree/", "/tree/internal"} {
		rec := h.get(h.repoPath(path), h.signedIn("alice"))
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Errorf("%s answered %d, want the tree screen", path, rec.Code)
		}
		if !strings.Contains(body, "no commits yet") {
			t.Errorf("%s does not say the repository is empty:\n%s", path, body)
		}
		if strings.Contains(body, "does not exist or you do not have access") {
			t.Errorf("%s says the repository is missing while showing it", path)
		}
		if !strings.Contains(body, `aria-label="Repository sections"`) {
			t.Errorf("%s dropped the repository's sections", path)
		}
	}
}

// TestNothingAtAnAddressInsideARepositoryKeepsTheSections asserts that a
// branch, path, commit or object that is not there, in a repository the
// reader has just been shown, is said in those words and under that
// repository's sections. The repository itself answered, so a sentence
// saying it does not exist is false, and a page with no way on is a dead
// end. The status stays 404 and the sentence still refuses to say whether
// the thing is absent or withheld, because Origo refuses to say so too.
func TestNothingAtAnAddressInsideARepositoryKeepsTheSections(t *testing.T) {
	repo := "/v1/repos/1f2e3d"
	cases := []struct {
		screen string
		path   string
		fail   string
	}{
		{"tree", "/tree/internal", repo + "/tree/main"},
		{"file", "/blob/README.md", repo + "/tree/main"},
		{"raw", "/raw/README.md", repo + "/blob/b1"},
		{"log", "/log", repo + "/commits"},
		{"commit", "/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d", repo + "/commits/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"},
		{"overview", "?ref=gone", repo + "/tree/gone"},
	}
	for _, tc := range cases {
		for _, code := range []int{http.StatusNotFound, http.StatusForbidden} {
			h := newHarness(t)
			h.fake.status[tc.fail] = code
			rec := h.get(h.repoPath(tc.path), h.signedIn("alice"))
			body := rec.Body.String()
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s with %s answering %d: status %d, want 404", tc.screen, tc.fail, code, rec.Code)
			}
			if strings.Contains(body, "This repository does not exist") {
				t.Errorf("%s with %s answering %d says the repository is missing while showing it", tc.screen, tc.fail, code)
			}
			if !strings.Contains(body, "does not exist or you do not have access") {
				t.Errorf("%s with %s answering %d does not keep the one sentence:\n%s", tc.screen, tc.fail, code, body)
			}
			if !strings.Contains(body, `aria-label="Repository sections"`) {
				t.Errorf("%s with %s answering %d dropped the repository's sections", tc.screen, tc.fail, code)
			}
			if !strings.Contains(body, `href="`+h.repoPath("/refs")+`"`) {
				t.Errorf("%s with %s answering %d offers no way to the references", tc.screen, tc.fail, code)
			}
		}
	}
}

// TestARepositoryThatIsNotThereLeadsBack asserts that the page for a
// repository the reader cannot open offers the one place they can go.
func TestARepositoryThatIsNotThereLeadsBack(t *testing.T) {
	h := newHarness(t)
	h.fake.status["/v1/repos/1f2e3d"] = http.StatusNotFound
	page := doc(t, h.get(h.repoPath(""), h.signedIn("alice")).Body.String())
	var back bool
	for _, a := range elements(elements(page, "main")[0], "a") {
		if attr(a, "href") == "/" {
			back = true
		}
	}
	if !back {
		t.Error("the not-found page has no link back to the repositories")
	}
}
