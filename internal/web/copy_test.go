// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"
)

// TestTheQuotableValuesSitInOneDisclosure asserts the copy list.
//
// A value an agent is asked to quote back is an address, a hash or a path,
// and marking each one where it stands puts a row of identical unlabelled
// controls down the reading column. They are collected into one disclosure
// instead: shut until a reader asks, one label a value, and every field
// readonly, so the platform's own copy key is the whole interaction.
//
// Nothing in it is a control that needs a script. There is no copy button
// anywhere in this interface, because a button that copies cannot work here.
func TestTheQuotableValuesSitInOneDisclosure(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	const head = "9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"

	for screen, want := range map[string][]string{
		h.repoPath("/blob/README.md"): {"Permalink", "Commit", "Path", "Clone address"},
		h.repoPath("/commit/" + head): {"Commit", "Patch address", "Tree at this commit", "Clone address"},
	} {
		page := doc(t, h.get(screen, c).Body.String())
		box := classed(page, "copy")
		if len(box) != 1 {
			t.Fatalf("%s carries %d copy disclosures, want one", screen, len(box))
		}
		if hasAttr(box[0], "open") {
			t.Errorf("%s: the copy list is open before a reader asks", screen)
		}
		if box[0].Data != "details" {
			t.Errorf("%s: the copy list is a <%s>, want the element that opens with no script", screen, box[0].Data)
		}

		var got []string
		ids := map[string]bool{}
		for _, in := range elements(box[0], "input") {
			if !hasAttr(in, "readonly") {
				t.Errorf("%s: a quotable value is editable", screen)
			}
			id := attr(in, "id")
			if id == "" || ids[id] {
				t.Errorf("%s: a field has no id of its own: %q", screen, id)
			}
			ids[id] = true
			if attr(in, "value") == "" {
				t.Errorf("%s: the field for %q is empty", screen, id)
			}
		}
		for _, l := range elements(box[0], "label") {
			got = append(got, text(l))
			if !ids[attr(l, "for")] {
				t.Errorf("%s: a label points at no field: %q", screen, text(l))
			}
		}
		if strings.Join(got, ", ") != strings.Join(want, ", ") {
			t.Errorf("%s lists %v, want %v", screen, got, want)
		}
		if n := len(elements(box[0], "button")); n != 0 {
			t.Errorf("%s: the copy list carries %d buttons, and a button that copies needs a script", screen, n)
		}
	}

	// The permalink is the whole address and names the commit the read
	// resolved to, so it holds after the next push.
	page := doc(t, h.get(h.repoPath("/blob/README.md"), c).Body.String())
	var permalink string
	for _, in := range elements(classed(page, "copy")[0], "input") {
		if attr(in, "id") == "copy-1" {
			permalink = attr(in, "value")
		}
	}
	if !strings.HasPrefix(permalink, "https://code.example/") || !strings.Contains(permalink, head) {
		t.Errorf("the permalink is not a whole address at a commit: %q", permalink)
	}

	// A value the installation cannot supply leaves no empty field behind.
	h.fake.repo.Head = ""
	page = doc(t, h.get(h.repoPath("/blob/README.md"), c).Body.String())
	for _, l := range elements(classed(page, "copy")[0], "label") {
		if text(l) == "Permalink" || text(l) == "Commit" {
			t.Errorf("a read that named no commit still offers %q", text(l))
		}
	}
}
