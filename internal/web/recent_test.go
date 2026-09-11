// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/latere-ai/origo-web/internal/origo"
)

// recentTable is the "Opened in this session" table on a page, nil when the
// page has none.
func recentTable(page *html.Node) *html.Node {
	for _, table := range elements(page, "table") {
		for _, c := range elements(table, "caption") {
			if strings.TrimSpace(text(c)) == "Opened in this session" {
				return table
			}
		}
	}
	return nil
}

// TestTheRecentListNamesTheRepository asserts that the list of what this
// session opened says which repositories those were. A row that read as a
// bare identifier named nothing a person recognises.
func TestTheRecentListNamesTheRepository(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
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
	for _, path := range []string{"/", "/tokens"} {
		table := recentTable(doc(t, h.get(path, c, recent).Body.String()))
		if table == nil {
			t.Fatalf("%s has no list of what this session opened", path)
		}
		links := elements(table, "a")
		if len(links) != 1 {
			t.Fatalf("%s lists %d repositories, want the one opened", path, len(links))
		}
		if got := strings.TrimSpace(text(links[0])); got != "infra/origo" {
			t.Errorf("%s names the repository %q, want its owner and name", path, got)
		}
	}
}

// TestTheRecentListIsAbsentWhereTheDirectoryIs asserts that an installation
// which lists every repository the reader may see does not list some of them
// a second time. The list of what this session opened is the way in on an
// installation that has no directory, and nothing more.
func TestTheRecentListIsAbsentWhereTheDirectoryIs(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
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
	repo := h.fake.repo
	h.answering(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/repos" {
			writeJSON(w, map[string]any{"repos": []origo.Repo{repo}, "next_cursor": ""})
			return
		}
		writeJSON(w, repo)
	})
	for _, path := range []string{"/", "/tokens"} {
		page := doc(t, h.get(path, c, recent).Body.String())
		if recentTable(page) != nil {
			t.Errorf("%s lists what this session opened beside the directory that already lists it", path)
		}
		if !strings.Contains(rendered(page), "origo") {
			t.Errorf("%s does not list the repository at all", path)
		}
	}
}
