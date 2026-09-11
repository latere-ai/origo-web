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
