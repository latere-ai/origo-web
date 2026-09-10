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
	pageBlock   = regexp.MustCompile(`(?s)\n\.page \{(.*?)\n\}`)
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
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
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

// TestTheShellIsTheViewportAndTheMeasureBoundsProse asserts the interface's
// one rule about width, so it cannot be traded away a screen at a time.
//
// The shell takes the viewport less a gutter: no screen is a column with
// empty space either side of it. The measure is a property of running text,
// not of the page, so it is set once, in characters, on the blocks that hold
// prose. A table, a tree, a commit log, a diff and a file are data rather
// than prose, and they get the whole shell.
func TestTheShellIsTheViewportAndTheMeasureBoundsProse(t *testing.T) {
	css := string(mustAsset(t, "app.css"))

	frame := pageBlock.FindStringSubmatch(css)
	if frame == nil {
		t.Fatal("the stylesheet has no page frame")
	}
	for _, banned := range []string{"max-width", "margin"} {
		if strings.Contains(frame[1], banned) {
			t.Errorf("the shell sets %s, so it is a column and not the viewport:\n%s", banned, frame[1])
		}
	}
	if !strings.Contains(frame[1], "padding:") {
		t.Errorf("the shell has no gutter:\n%s", frame[1])
	}

	// The measure is a count of characters, so it holds at any zoom and in
	// any typeface, and exactly one rule reads it.
	if !strings.Contains(css, "--measure: 78ch;") {
		t.Error("the measure is not a count of characters")
	}
	if got := strings.Count(css, "var(--measure)"); got != 1 {
		t.Errorf("%d rules bind something to the measure, want the one that binds prose", got)
	}
	if !strings.Contains(css, ".prose, .lead, .notice { max-width: var(--measure); }") {
		t.Error("the measure is bound to something other than prose")
	}

	// The screens made of data are never bounded: the tables that carry a
	// repository's tree, log, diff, files and tokens take the whole shell.
	// The two screens made of prose always are. A readme and a reference
	// table are prose that holds a table, and go with the words around it.
	data := map[string]bool{
		"home": true, "overview": true, "refs": true, "log": true, "commit": true,
		"compare": true, "tree": true, "file": true, "tokens": true,
	}
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		if !data[name] {
			continue
		}
		for _, table := range elements(doc(t, rec.Body.String()), "table") {
			if within(table, "readme") {
				continue
			}
			if within(table, "prose") {
				t.Errorf("%s: a table is bounded by the measure written for prose", name)
			}
		}
	}
	for _, path := range []string{"/sign-in", "/docs/agents"} {
		page := doc(t, h.get(path).Body.String())
		var prose int
		find(page, func(e *html.Node) {
			if strings.Contains(attr(e, "class"), "prose") {
				prose++
			}
		})
		if prose == 0 {
			t.Errorf("%s is a page of prose and nothing on it is bounded by the measure", path)
		}
	}
}
