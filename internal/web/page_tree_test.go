// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"
)

// TestATreeCountsWhatItShows asserts the line under a directory listing.
//
// Origo pages a tree by cursor and returns no total, so the line counts the
// rows on the screen. It sits beside the directory's own history, which is
// the answer to the column this screen does not have: the last commit of
// each entry is one call per row, and Origo offers no batch for it.
func TestATreeCountsWhatItShows(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")

	body := text(elements(doc(t, h.get(h.repoPath("/tree/internal"), c).Body.String()), "body")[0])
	if !strings.Contains(body, "1 entry") {
		t.Errorf("a directory of one entry does not count it: %q", body)
	}

	h.fake.trees["internal"] = manyEntries(120)
	page := doc(t, h.get(h.repoPath("/tree/internal"), c).Body.String())
	if body := text(elements(page, "body")[0]); !strings.Contains(body, "120 entries") {
		t.Errorf("a directory of 120 entries does not count them: %q", body)
	}

	var history string
	for _, nav := range classed(page, "pager") {
		for _, a := range elements(nav, "a") {
			if strings.Contains(text(a), "History") {
				history = attr(a, "href")
			}
		}
	}
	if !strings.Contains(history, "path=internal") {
		t.Errorf("the history link does not name this directory: %q", history)
	}
}
