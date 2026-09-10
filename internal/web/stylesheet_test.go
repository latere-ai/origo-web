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
	if !strings.Contains(text(elements(page, "body")[0]), "did not accept the credential") {
		t.Error("a refused reader is not told their credential was refused")
	}
	find(page, func(e *html.Node) {
		if style := attr(e, "style"); style != "" {
			t.Errorf("<%s> carries an inline style: %q", e.Data, style)
		}
	})

	// A notice is set apart by its surface and its border on all four
	// edges, which is the treatment a left rule was standing in for.
	for _, rule := range cssRules(css) {
		if strings.TrimSpace(rule.selector) != ".notice" {
			continue
		}
		for _, want := range []string{"border:", "background:", "border-radius:"} {
			if !strings.Contains(rule.body, want) {
				t.Errorf("a notice has no %s, so nothing but a rule sets it apart", strings.TrimSuffix(want, ":"))
			}
		}
	}
}

// TestTheBodyKeepsTheMastheadsEdges asserts the width rule of the interface:
// the body of a screen has the same left and right edges as the masthead, and
// nothing below the masthead stops short of them.
//
// A Go test cannot measure a rendered box, so it holds the stylesheet to the
// rules that would narrow one. The shell is the viewport less a gutter. No
// container between the masthead and the content sets a width of its own or
// centres itself. The measure binds text and never the box around it, which
// is the rule the two screens made of prose were breaking: a document capped
// at the measure took a little over half the window and left the rest empty.
func TestTheBodyKeepsTheMastheadsEdges(t *testing.T) {
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
	// any typeface, exactly one rule reads it, and everything that rule
	// names is a block of text rather than a box that holds one.
	if !strings.Contains(css, "--measure: 78ch;") {
		t.Error("the measure is not a count of characters")
	}
	text := map[string]bool{".prose": true, ".lead": true, ".notice": true, ".doc p": true}
	var bound int
	for _, rule := range cssRules(css) {
		if !strings.Contains(rule.body, "var(--measure)") {
			continue
		}
		bound++
		for sel := range strings.SplitSeq(rule.selector, ",") {
			if sel = strings.TrimSpace(sel); !text[sel] {
				t.Errorf("the measure binds %q, which is not a block of text", sel)
			}
		}
	}
	if bound != 1 {
		t.Errorf("%d rules bind something to the measure, want the one that binds text", bound)
	}

	// No box between the masthead and the content narrows itself or
	// centres itself. Either one is how half a window goes empty.
	boxes := map[string]bool{
		"main": true, ".page": true, ".gate": true, ".doc": true,
		".doc-body": true, ".panel": true, ".stack": true, ".fields": true,
	}
	for _, rule := range cssRules(css) {
		for sel := range strings.SplitSeq(rule.selector, ",") {
			if !boxes[strings.TrimSpace(sel)] {
				continue
			}
			for decl := range strings.SplitSeq(rule.body, ";") {
				decl = strings.TrimSpace(decl)
				name, value, _ := strings.Cut(decl, ":")
				switch strings.TrimSpace(name) {
				case "max-width":
					if strings.TrimSpace(value) != "100%" {
						t.Errorf("%s is capped at %s, so the body is narrower than the masthead", sel, value)
					}
				case "margin":
					if strings.Contains(value, "auto") {
						t.Errorf("%s centres itself, which empties the window on both sides", sel)
					}
				}
			}
		}
	}

	// The screens made of data are never bounded: the tables that carry a
	// repository's tree, log, diff, files and tokens take the whole shell.
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
				t.Errorf("%s: a table is bounded by the measure written for text", name)
			}
		}
	}

	// The two screens made of prose still hold a reading line, and the
	// documentation holds the far edge with the section list rather than
	// leaving it empty.
	for _, path := range []string{"/sign-in", "/docs/agents"} {
		page := doc(t, h.get(path).Body.String())
		var bounded int
		find(page, func(e *html.Node) {
			class := attr(e, "class")
			if strings.Contains(class, "prose") || strings.Contains(class, "lead") ||
				(e.Data == "p" && within(e, "doc")) {
				bounded++
			}
		})
		if bounded == 0 {
			t.Errorf("%s is a page of prose and no line on it is bounded", path)
		}
	}
	docs := doc(t, h.get("/docs/agents").Body.String())
	var sectionList *html.Node
	find(docs, func(e *html.Node) {
		if e.Data == "nav" && strings.Contains(attr(e, "class"), "toc") {
			sectionList = e
		}
	})
	if sectionList == nil {
		t.Fatal("the documentation has no section list")
	}
	if !within(sectionList, "doc-body") {
		t.Error("the section list is not beside the document, so nothing holds the far edge")
	}
}

// cssRule is one declaration block: what it selects and what it declares.
type cssRule struct{ selector, body string }

// cssRules splits the stylesheet into blocks. Comments go first, because a
// comment sits between a rule and the one above it and would otherwise be
// read as part of the selector. A block inside a media query is returned like
// any other; the query itself holds no declarations, so it does not match.
func cssRules(css string) []cssRule {
	var out []cssRule
	for _, m := range ruleBlock.FindAllStringSubmatch(cssComment.ReplaceAllString(css, ""), -1) {
		out = append(out, cssRule{selector: strings.TrimSpace(m[1]), body: m[2]})
	}
	return out
}

var (
	ruleBlock  = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// TestNoScreenCarriesANativeMenu asserts the rule the stylesheet writes down:
// the interface uses no select anywhere.
//
// A select's popup is drawn by the operating system, in the system's own
// highlight colour, and no rule in this stylesheet reaches it, so a screen
// with one is a screen the interface does not control. A fixed set of choices
// is a radio group; a long set is a radio group in a box that scrolls. The
// browser gives both arrow-key movement with no script.
func TestNoScreenCarriesANativeMenu(t *testing.T) {
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		for _, tag := range []string{"select", "option", "optgroup", "datalist"} {
			if got := len(elements(page, tag)); got != 0 {
				t.Errorf("%s carries %d <%s> elements, which the operating system draws", name, got, tag)
			}
		}
	}
	// The repository choice is the long set, so it is the group that
	// scrolls rather than one that runs down the page.
	page := doc(t, h.get(h.repoPath(""), h.signedIn("alice")).Body.String())
	var scrolling bool
	find(page, func(e *html.Node) {
		if strings.Contains(attr(e, "class"), "choices") && strings.Contains(attr(e, "class"), "scrolling") {
			scrolling = true
		}
	})
	if !scrolling {
		t.Error("the reference choice is not in a box that scrolls, so a repository with many branches runs off the page")
	}
	css := string(mustAsset(t, "app.css"))
	if strings.Contains(css, "appearance:") {
		t.Error("the stylesheet tries to skin a native control instead of not using one")
	}
}

// TestTextThatDoesTheSameJobLooksTheSame asserts the role system: the
// interface has a small set of roles for text, each with one class, and no
// screen invents a variant of one.
//
// An option is the case that kept drifting. A title glued to its description
// with a dash lays out differently from one without a description, and the
// same choice read differently on two screens. Title and note are two
// elements the stylesheet lays out, so the pattern holds when a note is
// missing and reads the same wherever a choice is offered.
func TestTextThatDoesTheSameJobLooksTheSame(t *testing.T) {
	css := string(mustAsset(t, "app.css"))
	// One rule gives a role its treatment. The shared width rule names
	// several selectors and gives none of them a look, so it is not one.
	for _, role := range []string{".hint", ".option-title", ".option-note", ".lead", ".notice"} {
		var defined int
		for _, rule := range cssRules(css) {
			if strings.TrimSpace(rule.selector) == role {
				defined++
			}
		}
		if defined != 1 {
			t.Errorf("%s is styled by %d rules of its own, want the one rule the role has", role, defined)
		}
	}

	h := newHarness(t)
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		find(page, func(e *html.Node) {
			if !strings.Contains(attr(e, "class"), "choice") || strings.Contains(attr(e, "class"), "choices") {
				return
			}
			var titles, notes int
			find(e, func(c *html.Node) {
				switch attr(c, "class") {
				case "option-title":
					titles++
				case "option-note":
					notes++
				}
			})
			if titles != 1 {
				t.Errorf("%s: a choice carries %d titles, want one", name, titles)
			}
			if notes > 1 {
				t.Errorf("%s: a choice carries %d notes, want at most one", name, notes)
			}
			if len(elements(e, "strong")) > 0 {
				t.Errorf("%s: a choice marks its title by hand instead of by role", name)
			}
		})
		// A hint under a field is the hint role, never the class the
		// interface uses for incidental detail elsewhere.
		find(page, func(e *html.Node) {
			if attr(e, "class") == "meta" && (within(e, "field") || within(e, "fields")) {
				t.Errorf("%s: a field hint is styled by hand instead of by role: %q", name, text(e))
			}
		})
		// Nothing on any screen carries a style of its own.
		find(page, func(e *html.Node) {
			if attr(e, "style") != "" {
				t.Errorf("%s: <%s> carries an inline style", name, e.Data)
			}
		})
	}
}

// paddedSurface names the enclosing surface that sets padding of its own,
// empty when there is none. A flush panel sets none: its content starts at its
// edge, so a block inside it is the panel's content and not a box within a
// box.
func paddedSurface(n *html.Node) string {
	for p := n.Parent; p != nil; p = p.Parent {
		class := attr(p, "class")
		if strings.Contains(class, "panel") && !strings.Contains(class, "flush") {
			return "panel"
		}
		if strings.Contains(class, "doc") && !strings.Contains(class, "doc-body") {
			return "document"
		}
	}
	return ""
}

// TestOneLeftEdgeHoldsOnEveryScreen asserts the alignment rule: every heading
// and every first line of text on a screen starts at the same left edge.
//
// The rule that broke it was a box inside a box. A notice carries its own
// surface and its own padding, so a notice used as the body of a panel or of
// a document started its text about 35px right of the heading in the panel
// above it. A notice stands on its own in the page; inside a panel or a
// document, what it would say is said in that surface's own voice.
//
// The diff is the one exception, and it is not one: a file over the render
// budget replaces its table with an explanation, and there is no heading
// beside it to share an edge with.
func TestOneLeftEdgeHoldsOnEveryScreen(t *testing.T) {
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		find(page, func(e *html.Node) {
			if !strings.Contains(attr(e, "class"), "notice") {
				return
			}
			if box := paddedSurface(e); box != "" {
				t.Errorf("%s: a notice sits inside a %s that pads its own content, so the notice starts right of the heading above it: %q",
					name, box, text(e))
			}
		})
	}

	// The surfaces that do carry padding pad every child alike, so the
	// heading and the first line of a panel start together.
	css := string(mustAsset(t, "app.css"))
	for _, rule := range cssRules(css) {
		sel := strings.TrimSpace(rule.selector)
		if sel != ".panel" && sel != ".doc" {
			continue
		}
		for decl := range strings.SplitSeq(rule.body, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(decl), ":")
			if strings.TrimSpace(name) == "text-indent" {
				t.Errorf("%s indents its text, so its children do not share an edge", sel)
			}
		}
	}
}
