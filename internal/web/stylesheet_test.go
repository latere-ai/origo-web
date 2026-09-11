// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
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
//
// It names the families that are not colours and treats everything else as
// one, rather than the other way round. A list of colour names goes quietly
// out of date the moment the palette is renamed, and a token that has dropped
// off it is a token no longer held to having a value in both themes, which is
// the failure this whole test exists to catch.
func isColour(token string) bool {
	for _, notColour := range []string{"--font-", "--t-", "--s-", "--radius", "--row", "--measure", "--tap"} {
		if strings.HasPrefix(token, notColour) {
			return false
		}
	}
	return true
}

// headingRoles are the roles a heading element may wear, with the typeface
// each is set in. Monospace is on this list exactly once, for the role whose
// content is a machine string: a repository's owner and slug, a path, a
// filename. A sentence never wears it.
var headingRoles = map[string]string{
	".heading-page":    "sans",
	".heading-section": "sans",
	".heading-sub":     "sans",
	".heading-path":    "mono",
	".label":           "sans",
}

// TestAHeadingLooksLikeItsRole asserts the half of the role system the class
// rules alone do not cover: a heading element carries document structure, a
// role class carries the look, and no screen reaches in to change the look of
// a heading it happens to hold.
//
// This is the assertion that was missing. The role test below held the role
// classes to one rule each, so the classes could not drift, but a heading was
// styled by its element name and any screen could override that with a
// descendant selector. `.gate h1` did exactly that: it set the sign-in
// heading in monospace at twice the size of the two headings beside it, all
// three of them naming a panel, and every existing assertion passed because
// none of them looked at a heading at all.
func TestAHeadingLooksLikeItsRole(t *testing.T) {
	css := string(mustAsset(t, "app.css"))

	// One rule gives a role its treatment, and it is the rule whose whole
	// selector is that role. A width breakpoint may step a role, which is
	// the role changing with the screen's width and not one screen's
	// opinion of it, so the count is taken outside the media blocks.
	for role := range headingRoles {
		var defined int
		for _, rule := range cssRules(outsideMedia(css)) {
			for sel := range strings.SplitSeq(rule.selector, ",") {
				if strings.TrimSpace(sel) == role {
					defined++
				}
			}
		}
		if defined != 1 {
			t.Errorf("%s is styled by %d rules of its own, want the one rule the role has", role, defined)
		}
	}

	// Nothing else sets how a heading looks. A bare element selector is the
	// shared baseline and a width breakpoint may step a role, but a selector
	// that reaches a heading through a screen is the defect above. Rendered
	// Markdown is authored content rather than chrome and sets its own scale.
	looks := []string{"font-family", "font-size", "font-weight", "letter-spacing", "line-height"}
	for _, rule := range cssRules(css) {
		for sel := range strings.SplitSeq(rule.selector, ",") {
			sel = strings.TrimSpace(sel)
			if !headingSelector(sel) || strings.HasPrefix(sel, ".readme") {
				continue
			}
			if _, isRole := headingRoles[sel]; isRole || bareElements(sel) {
				continue
			}
			for _, look := range looks {
				if strings.Contains(rule.body, look+":") {
					t.Errorf("%q sets %s on a heading, so a heading looks one way on that screen and another everywhere else: %s",
						sel, look, strings.TrimSpace(rule.body))
				}
			}
		}
	}

	// Monospace on a heading is the machine-string role and nothing else.
	for _, rule := range cssRules(css) {
		if !strings.Contains(rule.body, "var(--font-mono)") {
			continue
		}
		for sel := range strings.SplitSeq(rule.selector, ",") {
			sel = strings.TrimSpace(sel)
			if headingRoles[sel] == "mono" || strings.HasPrefix(sel, ".readme") {
				continue
			}
			if headingSelector(sel) {
				t.Errorf("%q sets a heading in monospace, which is the interface's signal that a string is a hash, a path or a filename", sel)
			}
		}
	}

	// And every heading a reader meets wears exactly one of the roles, so
	// none of them falls back to a size nobody chose.
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		page := doc(t, rec.Body.String())
		for _, tag := range []string{"h1", "h2", "h3", "h4"} {
			for _, e := range elements(page, tag) {
				if within(e, "readme") {
					continue
				}
				var worn int
				for class := range strings.FieldsSeq(attr(e, "class")) {
					if _, ok := headingRoles["."+class]; ok {
						worn++
					}
				}
				if worn != 1 {
					t.Errorf("%s: <%s>%s</%s> wears %d heading roles, want one", name, tag, text(e), tag, worn)
				}
			}
		}
	}
}

// outsideMedia is the stylesheet with its media blocks removed, which is the
// stylesheet as it applies at every width.
func outsideMedia(css string) string {
	var out strings.Builder
	for {
		i := strings.Index(css, "@media")
		if i < 0 {
			out.WriteString(css)
			return out.String()
		}
		out.WriteString(css[:i])
		depth, j := 0, i
		for ; j < len(css); j++ {
			switch css[j] {
			case '{':
				depth++
			case '}':
				if depth--; depth == 0 {
					j++
					goto done
				}
			}
		}
	done:
		css = css[min(j, len(css)):]
	}
}

// headingSelector reports whether a selector reaches a heading element.
func headingSelector(sel string) bool {
	for _, tag := range []string{"h1", "h2", "h3", "h4"} {
		for part := range strings.FieldsSeq(strings.ReplaceAll(sel, ">", " ")) {
			if part == tag || strings.HasPrefix(part, tag+":") || strings.HasPrefix(part, tag+".") {
				return true
			}
		}
	}
	for role := range headingRoles {
		if sel == role {
			return true
		}
	}
	return false
}

// bareElements reports whether a selector names element types and nothing
// else, which is the interface's shared baseline rather than one screen's
// opinion of a heading.
func bareElements(sel string) bool {
	if strings.ContainsAny(sel, ".#[:>") {
		return false
	}
	return len(strings.Fields(sel)) == 1
}

// contrast is the WCAG 2.1 ratio between two sRGB hex colours.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	v := strings.TrimPrefix(hex, "#")
	channel := func(i int) float64 {
		n, err := strconv.ParseUint(v[i:i+2], 16, 8)
		if err != nil {
			return 0
		}
		c := float64(n) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(0) + 0.7152*channel(2) + 0.0722*channel(4)
}

// inkFloors are the contrast a tone has to hold against every surface it is
// set on. The body ink is held to 7:1 and the secondary ink to the 4.5:1 the
// guidelines put on ordinary text.
//
// The muted tone is not here, and that is the point. Latere v2's --text-muted
// is #a0a0a0, which is 2.6:1 on a white surface. It cannot carry a line
// number, a column header, a breadcrumb or a timestamp, because those are
// read, and the light theme this replaced was doing exactly that. It marks
// instead: the separator in a path, the inert half of a pager, a disclosure
// arrow. mutedIsDecoration below holds it to that.
var inkFloors = map[string]float64{"--ink": 7, "--ink-2": 4.5}

// mutedUses are the rules allowed to read the muted tone. Each one is a mark
// rather than a word: nothing a reader has to read is in this list.
var mutedUses = map[string]bool{
	".muted":                       true,
	"button:hover":                 true,
	".button:hover":                true,
	".diff-file > summary::before": true,
	".picker > summary::after":     true,
	".pager .inert":                true,
}

// TestBothThemesMeetTheirContrast measures the palette instead of trusting it.
//
// A ratio is not something a page can be eyeballed into having, and the light
// theme went out with its third ink at 2.6:1 on a panel while that ink was
// carrying every line number, every column header and every breadcrumb.
// Nothing in the suite noticed. This computes every pair the interface
// actually paints, in both themes, from the token values themselves.
func TestBothThemesMeetTheirContrast(t *testing.T) {
	css := string(mustAsset(t, "app.css"))
	light, dark := lightBlock.FindStringSubmatch(css), darkBlock.FindStringSubmatch(css)
	if light == nil || dark == nil {
		t.Fatal("the stylesheet has no light block or no dark block")
	}
	base := themeValues(light[1])
	grounds := []string{"--bg", "--surface", "--raised"}
	for _, theme := range []struct {
		name   string
		values map[string]string
	}{
		{"light", base},
		{"dark", inherit(base, themeValues(dark[1]))},
	} {
		v := theme.values
		measure := func(fg, bg string, min float64) {
			t.Helper()
			a, b := solid(v, fg, v["--surface"]), solid(v, bg, v["--surface"])
			if a == "" || b == "" {
				t.Errorf("%s: %s on %s is not a pair of colours", theme.name, fg, bg)
				return
			}
			if got := contrast(a, b); got < min {
				t.Errorf("%s: %s on %s is %.2f:1, want %.1f:1 or better", theme.name, fg, bg, got, min)
			}
		}
		// Every ink on every surface it is ever set on.
		for ink, floor := range inkFloors {
			for _, ground := range grounds {
				measure(ink, ground, floor)
			}
		}
		// The accent carries every link and the one filled control.
		measure("--accent", "--bg", 4.5)
		measure("--accent", "--surface", 4.5)
		measure("--accent", "--raised", 4.5)
		measure("--accent-ink", "--accent", 4.5)
		// The diff, where the tint is laid over a panel: its own ink and the
		// body ink both have to survive it.
		measure("--add", "--add-bg", 4.5)
		measure("--del", "--del-bg", 4.5)
		measure("--ink", "--add-bg", 4.5)
		measure("--ink", "--del-bg", 4.5)
		// And the one inverted block, which paints the ground on the ink.
		measure("--bg", "--ink", 7)

		// A panel has an edge. The two surfaces of this palette are a
		// twentieth of a stop apart, so the frame is what a reader sees.
		if got := contrast(solid(v, "--line-2", v["--surface"]), v["--surface"]); got < 1.2 {
			t.Errorf("%s: the frame is %.2f:1 against a panel, so a panel has no edge", theme.name, got)
		}
	}

	// The muted tone marks and does not inform, so only the marks read it.
	for _, rule := range cssRules(outsideMedia(css)) {
		if !strings.Contains(rule.body, "var(--ink-3)") {
			continue
		}
		for sel := range strings.SplitSeq(rule.selector, ",") {
			if sel = strings.TrimSpace(sel); !mutedUses[sel] {
				t.Errorf("%q sets the muted tone, which is 2.6:1 on a panel and cannot carry anything a reader has to read", sel)
			}
		}
	}
}

// themeValues reads the colours a :root block defines, hex and rgba alike.
func themeValues(block string) map[string]string {
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{6}|rgba\([^)]*\))\s*;`).FindAllStringSubmatch(block, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// solid is a token as a reader sees it: a hex value as itself, and a
// translucent one composited over the surface it is painted on, because a
// tint at 10% black is not a colour until something is behind it.
func solid(v map[string]string, token, over string) string {
	raw, ok := v[token]
	if !ok {
		return ""
	}
	if strings.HasPrefix(raw, "#") {
		return raw
	}
	m := regexp.MustCompile(`rgba\(\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9.]+)\s*\)`).FindStringSubmatch(raw)
	if m == nil || !strings.HasPrefix(over, "#") {
		return ""
	}
	alpha, err := strconv.ParseFloat(m[4], 64)
	if err != nil {
		return ""
	}
	out := "#"
	for i := range 3 {
		top, err := strconv.ParseUint(m[i+1], 10, 8)
		if err != nil {
			return ""
		}
		under, err := strconv.ParseUint(strings.TrimPrefix(over, "#")[i*2:i*2+2], 16, 8)
		if err != nil {
			return ""
		}
		out += fmt.Sprintf("%02x", uint8(alpha*float64(top)+(1-alpha)*float64(under)+0.5))
	}
	return out
}

// inherit is the dark theme as a browser reads it: the light block's values
// with the dark block's laid over them.
func inherit(base, over map[string]string) map[string]string {
	out := map[string]string{}
	maps.Copy(out, base)
	maps.Copy(out, over)
	return out
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
	// so removing the rule did not leave it undistinguishable. The assertion
	// is the treatment and not the exact declarations: a token may be renamed
	// and the radius scale may collapse to one value, and a block that lost
	// its surface or grew a partial border must still fail here.
	for _, role := range []string{".notice", ".readme blockquote"} {
		var found bool
		for _, rule := range cssRules(css) {
			if strings.TrimSpace(rule.selector) != role {
				continue
			}
			found = true
			for _, want := range []string{"background:", "border:"} {
				if !strings.Contains(rule.body, want) {
					t.Errorf("%s has no %s, so nothing but a rule would set it apart:\n%s",
						role, strings.TrimSuffix(want, ":"), rule.body)
				}
			}
			for decl := range strings.SplitSeq(rule.body, ";") {
				name, _, _ := strings.Cut(strings.TrimSpace(decl), ":")
				if n := strings.TrimSpace(name); strings.HasPrefix(n, "border-") && n != "border-radius" {
					t.Errorf("%s draws %s, so its border is not on all four edges", role, n)
				}
			}
		}
		if !found {
			t.Errorf("%s is not styled at all", role)
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
	if !strings.Contains(text(elements(page, "body")[0]), "You are signed out") {
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
	//
	// The front door is the one exception, and it is written down here
	// rather than left to a selector nobody listed. Every other screen holds
	// the far edge with a table, a tree, a log, a diff or a document's
	// samples. The door has a button and three short paragraphs: at a
	// desktop width its sentences stop at the measure and the rest of the
	// window is empty beside them, which is the composition this rule exists
	// to prevent. So the door is a column the window sits either side of,
	// and it is bounded once, by `.door`, which is asserted below.
	boxes := map[string]bool{
		"main": true, ".page": true, ".gate": true, ".doc": true,
		".doc-body": true, ".panel": true, ".stack": true, ".fields": true,
		".door": true,
	}
	for _, rule := range cssRules(css) {
		for sel := range strings.SplitSeq(rule.selector, ",") {
			if strings.TrimSpace(sel) == ".door" {
				continue
			}
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

	// The exception is one box on one screen, and it is a column with the
	// window either side of it rather than a body that stops short of the
	// masthead on one side.
	var doors int
	for _, rule := range cssRules(css) {
		if strings.TrimSpace(rule.selector) != ".door" {
			continue
		}
		doors++
		if !strings.Contains(rule.body, "max-width:") {
			t.Error("the front door's column is not bounded")
		}
	}
	if doors != 1 {
		t.Errorf("%d rules bound the front door's column, want one", doors)
	}
	for _, rule := range cssRules(css) {
		if strings.TrimSpace(rule.selector) != ".gate" {
			continue
		}
		if !strings.Contains(rule.body, "justify-content: center") {
			t.Error("the front door's column is bounded without being centred, so the window empties on one side")
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
		t.Error("the section list is not beside the document")
	}

	// It is the first column, and it is first because running text stops
	// at the measure: a column sized by anything else leaves the
	// difference empty, and that emptiness must fall after the last piece
	// of content rather than between two of them. With the list first,
	// the words begin one gutter from where it ends.
	var body *html.Node
	find(docs, func(e *html.Node) {
		if strings.Contains(attr(e, "class"), "doc-body") {
			body = e
		}
	})
	if body == nil {
		t.Fatal("the documentation has no two-column body")
	}
	var firstColumn *html.Node
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			firstColumn = c
			break
		}
	}
	if firstColumn != sectionList {
		t.Errorf("the first column of the document is <%s>, so the words are separated from the list by whatever the column does not use",
			firstColumn.Data)
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

// TestTheRadiiAreSmall holds every corner in the interface to 4px or less.
// A large radius and a pill-shaped control spend space around the content
// and read as decoration, and the interface is compact. Every token on the
// radius scale and every literal border-radius is checked, so a rule that
// bypasses the scale is caught as well as a scale that grows.
func TestTheRadiiAreSmall(t *testing.T) {
	const maxPx = 4
	css := string(mustAsset(t, "app.css"))
	var checked int
	for _, rule := range cssRules(css) {
		for decl := range strings.SplitSeq(rule.body, ";") {
			name, value, ok := strings.Cut(decl, ":")
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if !ok || (name != "border-radius" && !strings.HasPrefix(name, "--radius-")) {
				continue
			}
			if strings.HasPrefix(value, "var(--radius-") || value == "0" {
				continue
			}
			px, err := strconv.Atoi(strings.TrimSuffix(value, "px"))
			if err != nil {
				t.Errorf("%s: %s is %q, want a pixel value", rule.selector, name, value)
				continue
			}
			checked++
			if px > maxPx {
				t.Errorf("%s: %s is %dpx, want %dpx or less", rule.selector, name, px, maxPx)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no radius was found to check")
	}
}

// TestTheSurfacesAreCompact holds the spacing that sets the density of a
// screen. A table row, a control, a panel and the page frame each take at
// most one named step of the spacing scale as their vertical padding, so a
// window shows as many rows as it has room for and a form does not spread
// over it. The bound is the step and not the pixel, so the scale itself can
// move without moving this test.
func TestTheSurfacesAreCompact(t *testing.T) {
	css := string(mustAsset(t, "app.css"))
	bounds := map[string]int{
		"th, td":          1, // a table row
		"button, .button": 2, // a control
		"input, textarea": 2,
		".panel":          4, // a panel's own padding
		".page":           4, // the page frame's top
	}
	for selector, maxStep := range bounds {
		var found bool
		for _, rule := range cssRules(outsideMedia(css)) {
			if strings.TrimSpace(rule.selector) != selector {
				continue
			}
			found = true
			var padding string
			for decl := range strings.SplitSeq(rule.body, ";") {
				if name, value, ok := strings.Cut(decl, ":"); ok && strings.TrimSpace(name) == "padding" {
					padding = strings.TrimSpace(value)
				}
			}
			if padding == "" {
				t.Errorf("%s sets no padding", selector)
				continue
			}
			// The first value of the shorthand is the top, which is what
			// stacks down a screen.
			top := strings.Fields(padding)[0]
			step, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(top, "var(--s-"), ")"))
			if err != nil {
				t.Errorf("%s pads %q, want a step of the spacing scale", selector, top)
				continue
			}
			if step > maxStep {
				t.Errorf("%s pads its top by --s-%d, want --s-%d or less", selector, step, maxStep)
			}
		}
		if !found {
			t.Errorf("%s is not styled", selector)
		}
	}
}
