// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestAMachineViewIsTheSameAddressPlusASuffix asserts the one rule the
// machine line follows, so a program finds it by shape.
//
// It is the last block of the page, it begins with the word machine, and
// every link after it is named by its format. Each address is this same
// service plus a suffix and answers what the link says it does. The same
// addresses appear in the head, so a program that knows the convention never
// loads the document.
//
// A screen with no machine view carries no line. This interface renders what
// Origo publishes and publishes no representation of its own, so the formats
// here are the ones Origo already serves.
func TestAMachineViewIsTheSameAddressPlusASuffix(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")

	for _, tc := range []struct {
		screen, path, format string
		status               int
	}{
		{"file", h.repoPath("/blob/README.md"), "raw", http.StatusOK},
		{"commit", h.repoPath("/commit/9f3c1abf20d4e7c8b5a1930fe6d2c4471be08a3d"), "patch", http.StatusOK},
	} {
		page := doc(t, h.get(tc.path, c).Body.String())
		line := classed(page, "machine")
		if len(line) != 1 {
			t.Fatalf("%s carries %d machine lines, want one", tc.screen, len(line))
		}
		if !strings.HasPrefix(text(line[0]), "machine") {
			t.Errorf("%s: the line does not begin with the word machine: %q", tc.screen, text(line[0]))
		}
		links := elements(line[0], "a")
		if len(links) != 1 {
			t.Fatalf("%s offers %d machine views, want one", tc.screen, len(links))
		}
		if got := text(links[0]); got != tc.format {
			t.Errorf("%s names its machine view %q, want the format %q", tc.screen, got, tc.format)
		}
		href := attr(links[0], "href")
		if !strings.HasPrefix(href, h.repoPath("")) {
			t.Errorf("%s: the machine view is not this same service: %q", tc.screen, href)
		}
		if rec := h.get(href, c); rec.Code != tc.status {
			t.Errorf("%s: the %s view answered %d", tc.screen, tc.format, rec.Code)
		}

		// And the head says the same, so a program never loads the page.
		var head string
		for _, l := range elements(page, "link") {
			if attr(l, "rel") == "alternate" {
				head = attr(l, "href")
			}
		}
		if head != href {
			t.Errorf("%s: the head names %q and the foot names %q", tc.screen, head, href)
		}
	}

	// The screens with nothing to offer offer nothing, rather than a line
	// with no links under it.
	for _, screen := range []string{"/", h.repoPath(""), h.repoPath("/log"), h.repoPath("/refs"), "/docs/agents"} {
		if got := classed(doc(t, h.get(screen, c).Body.String()), "machine"); len(got) != 0 {
			t.Errorf("%s carries a machine line with no machine view behind it", screen)
		}
	}
}

// TestTheMachineLineIsTheLastBlock holds the machine line to its place, which
// is what lets a program find it without reading the page.
func TestTheMachineLineIsTheLastBlock(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")
	page := doc(t, h.get(h.repoPath("/blob/README.md"), c).Body.String())

	shell := classed(page, "page")
	if len(shell) != 1 {
		t.Fatalf("the page has %d shells, want one", len(shell))
	}
	var last *html.Node
	for n := shell[0].LastChild; n != nil; n = n.PrevSibling {
		if n.Type == html.ElementNode {
			last = n
			break
		}
	}
	if last == nil || attr(last, "class") != "machine" {
		t.Errorf("the last block of the page is %v, want the machine line", last)
	}
}
