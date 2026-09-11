// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestAFileSaysWhetherItsAddressMoves asserts the strip that separates a
// citation which holds from one which drifts.
//
// A file read at a branch shows other bytes after the next push. A file read
// at a commit shows the same bytes forever. The screen names which one the
// reader is on and offers the other, and the pinned address is built from
// the commit Origo named in its answer, so it costs no second call.
func TestAFileSaysWhetherItsAddressMoves(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	const head = "9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"

	page := doc(t, h.get(h.repoPath("/blob/README.md"), c).Body.String())
	strip := classed(page, "pin")
	if len(strip) != 1 {
		t.Fatalf("a file at a branch carries %d strips, want one", len(strip))
	}
	if line := text(strip[0]); !strings.Contains(line, "main") || !strings.Contains(line, "next push") {
		t.Errorf("the strip does not say the address moves: %q", line)
	}
	pin := linkIn(strip[0], "Pin to")
	if !strings.Contains(pin, "ref="+head) {
		t.Errorf("the pinned address does not name the commit Origo resolved: %q", pin)
	}

	// And the pinned page says the other half, and leads back.
	page = doc(t, h.get(pin, c).Body.String())
	strip = classed(page, "pin")
	if len(strip) != 1 {
		t.Fatalf("a pinned file carries %d strips, want one", len(strip))
	}
	line := text(strip[0])
	if !strings.Contains(line, "pinned 9f3c1ab") || !strings.Contains(line, "always shows these bytes") {
		t.Errorf("the pinned strip does not say the address holds: %q", line)
	}
	if back := linkIn(strip[0], "View current on main"); back == "" || strings.Contains(back, "ref=") {
		t.Errorf("the pinned page does not lead back to the branch: %q", back)
	}

	// An installation that names no commit has nothing to pin to, so there
	// is no strip rather than one that cannot be acted on.
	h.fake.repo.Head = ""
	if got := classed(doc(t, h.get(h.repoPath("/blob/README.md"), c).Body.String()), "pin"); len(got) != 0 {
		t.Errorf("a read that named no commit still drew %d strips", len(got))
	}
}

// TestAnObjectIDIsToldFromAName asserts what decides which half of the strip
// a page shows: a reference written as hex and long enough for git to take as
// an abbreviation, which also resolves to itself.
func TestAnObjectIDIsToldFromAName(t *testing.T) {
	for ref, want := range map[string]bool{
		"9f3c1ab": true,
		"9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d": true,
		"main":                  false,
		"v1.4.2":                false,
		"deadbee":               true,
		"deadbe":                false,
		"release":               false,
		"9F3C1AB":               false,
		"feature1":              false,
		strings.Repeat("a", 65): false,
	} {
		if got := isObjectID(ref); got != want {
			t.Errorf("isObjectID(%q) = %v, want %v", ref, got, want)
		}
	}
}

// linkIn is the address of the first link in n whose text starts with prefix.
func linkIn(n *html.Node, prefix string) string {
	for _, a := range elements(n, "a") {
		if strings.HasPrefix(text(a), prefix) {
			return attr(a, "href")
		}
	}
	return ""
}
