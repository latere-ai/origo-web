// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestTheLogPagerWalksOneWay asserts the commit log's pager.
//
// Origo's commit log is walked by a cursor that carries the last commit of
// the page, so the walk goes one way: there is a page after this one and
// there is no address for the page before it. The pager keeps both halves so
// the row does not change shape between pages, and the half that leads
// nowhere is text rather than a link. The link names how many commits it
// fetches, so a reader knows the size of the step.
func TestTheLogPagerWalksOneWay(t *testing.T) {
	h := newHarness(t)
	c := h.signedIn("alice")

	h.fake.commits = manyCommits(50)
	h.fake.nextCursor = "6b1e0d2"
	page := doc(t, h.get(h.repoPath("/log"), c).Body.String())

	older := pagerLinks(page)
	if len(older) != 1 {
		t.Fatalf("the pager offers %d links, want the one Origo's cursor can address: %v", len(older), older)
	}
	if !strings.Contains(older[0], "cursor=6b1e0d2") {
		t.Errorf("the older link does not carry the cursor: %q", older[0])
	}
	if label := pagerText(page); !strings.Contains(label, "Older 50") {
		t.Errorf("the older link does not name the size of its step: %q", label)
	}
	if !strings.Contains(pagerInert(page), "Newer") {
		t.Errorf("the newer half is missing or is a link: %q", pagerInert(page))
	}

	// The last page has nothing after it, so both halves are text and the
	// pager offers no address that repeats the page a reader is on.
	h.fake.nextCursor = ""
	page = doc(t, h.get(h.repoPath("/log"), c).Body.String())
	if links := pagerLinks(page); len(links) != 0 {
		t.Errorf("the last page still offers %v", links)
	}
	if inert := pagerInert(page); !strings.Contains(inert, "Newer") || !strings.Contains(inert, "Older") {
		t.Errorf("the last page's pager lost a half: %q", inert)
	}
}

// pagerLinks is every address the pager offers.
func pagerLinks(page *html.Node) []string {
	var out []string
	for _, nav := range classed(page, "pager") {
		for _, a := range elements(nav, "a") {
			out = append(out, attr(a, "href"))
		}
	}
	return out
}

// pagerText is everything the pager says.
func pagerText(page *html.Node) string {
	var out []string
	for _, nav := range classed(page, "pager") {
		out = append(out, text(nav))
	}
	return strings.Join(out, " ")
}

// pagerInert is what the pager says in the half that leads nowhere.
func pagerInert(page *html.Node) string {
	var out []string
	for _, nav := range classed(page, "pager") {
		for _, e := range classed(nav, "inert") {
			out = append(out, text(e))
		}
	}
	return strings.Join(out, " ")
}
