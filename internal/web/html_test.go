// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// doc parses a rendered page so a test asserts against its structure and not
// against a substring of its markup.
func doc(t *testing.T, body string) *html.Node {
	t.Helper()
	n, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse the page: %v", err)
	}
	return n
}

// find walks the document, calling visit on every element.
func find(n *html.Node, visit func(*html.Node)) {
	if n.Type == html.ElementNode {
		visit(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		find(c, visit)
	}
}

// elements returns every element with the given tag name.
func elements(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	find(n, func(e *html.Node) {
		if e.Data == tag {
			out = append(out, e)
		}
	})
	return out
}

// attr reads one attribute, empty when it is absent.
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// hasAttr reports whether an attribute is present, which is the only
// question worth asking of a boolean one: readonly, checked, open.
func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

// text is the element's text with its descendants' text, whitespace
// collapsed.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// within reports whether an element sits inside one carrying the given
// class, which is how a test tells the interface's own markup from the
// repository content it renders.
func within(n *html.Node, class string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if strings.Contains(attr(p, "class"), class) {
			return true
		}
	}
	return false
}

// textOutside is the page's text with every subtree carrying the given class
// left out. A readme says whatever the repository says, and that is not the
// interface speaking.
func textOutside(n *html.Node, class string) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode && strings.Contains(attr(x, "class"), class) {
			return
		}
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
