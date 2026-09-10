// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/latere-ai/origo-web/internal/origo"
)

var (
	tokenUse    = regexp.MustCompile(`var\((--[a-z0-9-]+)`)
	tokenDefine = regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+)\s*:`)
	lightBlock  = regexp.MustCompile(`(?s):root \{(.*?)\n\}`)
	darkBlock   = regexp.MustCompile(`(?s)@media \(prefers-color-scheme: dark\) \{\s*:root \{(.*?)\n  \}`)
)

// TestBothThemesAreComplete asserts the property a page cannot be eyeballed
// into having: every colour the interface uses is a token, every token it
// uses is defined, and every colour token has a value in both themes. A
// colour defined only in the light block is a page that goes unreadable in
// the dark one, and nothing but this catches it.
func TestBothThemesAreComplete(t *testing.T) {
	css := string(mustAsset(t, "app.css"))

	if strings.Count(css, "{") != strings.Count(css, "}") {
		t.Fatalf("the stylesheet has %d opening and %d closing braces",
			strings.Count(css, "{"), strings.Count(css, "}"))
	}

	defined := map[string]bool{}
	for _, m := range tokenDefine.FindAllStringSubmatch(css, -1) {
		defined[m[1]] = true
	}
	for _, m := range tokenUse.FindAllStringSubmatch(css, -1) {
		if !defined[m[1]] {
			t.Errorf("%s is used and never defined", m[1])
		}
	}

	light := lightBlock.FindStringSubmatch(css)
	dark := darkBlock.FindStringSubmatch(css)
	if light == nil || dark == nil {
		t.Fatal("the stylesheet has no light block or no dark block")
	}
	inDark := map[string]bool{}
	for _, m := range tokenDefine.FindAllStringSubmatch(dark[1], -1) {
		inDark[m[1]] = true
	}
	for _, m := range tokenDefine.FindAllStringSubmatch(light[1], -1) {
		name := m[1]
		if !isColour(name) || inDark[name] {
			continue
		}
		t.Errorf("%s has a light value and no dark one", name)
	}

	// The theme follows the system setting alone. A switch would need a
	// script or a cookie, and both are out.
	if strings.Contains(css, "data-theme") {
		t.Error("the stylesheet carries a theme switch")
	}
	if !strings.Contains(css, "color-scheme: light dark") {
		t.Error("the page does not tell the browser it has both themes")
	}
}

// isColour reports whether a token names a colour rather than a size, a
// radius, or a typeface.
func isColour(token string) bool {
	for _, part := range []string{"bg", "fg", "accent", "border", "diff", "focus"} {
		if strings.Contains(token, part) {
			return true
		}
	}
	return false
}

// TestTheNarrowRulesAreThere asserts the structural half of the narrow-width
// criterion: one breakpoint, data tables that restack, and code and diffs
// that scroll inside their own box rather than reflowing, because a wrapped
// line of source is a lie about the file.
func TestTheNarrowRulesAreThere(t *testing.T) {
	css := string(mustAsset(t, "app.css"))
	if n := strings.Count(css, "@media (max-width"); n != 1 {
		t.Errorf("the stylesheet has %d width breakpoints, want one", n)
	}
	for _, want := range []string{
		"table.stackable",  // data tables restack
		".scroller",        // wide content scrolls inside its own box
		"overflow-x: auto", //
		"--tap: 44px",      // tap targets
		"max-width: 100%",  // nothing forces the document wider than the screen
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the stylesheet has no %q", want)
		}
	}

	// Every wide table on every screen is inside a scrolling container, so
	// the document itself never scrolls sideways.
	h := newHarness(t)
	c := h.signedIn("alice")
	for name, path := range h.screens() {
		page := doc(t, h.get(path, c).Body.String())
		for _, table := range elements(page, "table") {
			var boxed bool
			for p := table.Parent; p != nil; p = p.Parent {
				if strings.Contains(attr(p, "class"), "scroller") ||
					strings.Contains(attr(p, "class"), "diff-body") {
					boxed = true
					break
				}
			}
			if !boxed {
				t.Errorf("%s: a table that can push the document sideways", name)
			}
		}
	}
}

// TestNoBlockIsMarkedByALeftRule asserts that nothing on any screen is set
// apart by a bar down its left edge. A notice, a callout, a quotation and a
// status block are distinguishable by their surface and their border, which
// read the same in both themes and at any zoom; a rule on one edge is an
// accent the interface does not have.
//
// The diff's own sign column is not a block and keeps its inset mark: it
// sits under a literal + or -, and it is the one place the interface tints.
func TestNoBlockIsMarkedByALeftRule(t *testing.T) {
	css := string(mustAsset(t, "app.css"))
	for _, banned := range []string{"border-left", "border-inline-start"} {
		if strings.Contains(css, banned) {
			t.Errorf("the stylesheet still sets %s on something", banned)
		}
	}

	// Every block that reads as a notice keeps a surface and a full border,
	// so removing the rule did not leave it undistinguishable.
	for _, rule := range []string{
		".notice {\n  border: 1px solid var(--border-strong);\n  border-radius: var(--radius-sm);\n  background: var(--bg-raised);",
		".readme blockquote {\n  padding: var(--s-3) var(--s-4);\n  background: var(--bg-raised);\n  border: 1px solid var(--border);",
	} {
		if !strings.Contains(css, rule) {
			t.Errorf("a block lost the surface and border that distinguish it:\n%s", rule)
		}
	}

	// And the notice is still a block on a real page, so the assertion
	// above is about something a reader sees, and it carries no rule of its
	// own inline either.
	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated"}}`))
	}))
	defer refuse.Close()
	h := newHarness(t)
	h.server = New(Options{
		Config: h.cfg, Sessions: mustSessions(t, h.cfg), API: origo.New(mustURL(t, refuse.URL), refuse.Client()),
	})
	page := doc(t, h.get("/", h.signedIn("alice")).Body.String())
	var notices int
	find(page, func(e *html.Node) {
		if !strings.Contains(attr(e, "class"), "notice") {
			return
		}
		notices++
		if style := attr(e, "style"); style != "" {
			t.Errorf("a notice carries an inline style: %q", style)
		}
	})
	if notices != 1 {
		t.Errorf("the signed-out page rendered %d notice blocks, want the one that says so", notices)
	}
}
