// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// firstWithClass is the first element on the page carrying the class.
func firstWithClass(page *html.Node, class string) *html.Node {
	var out *html.Node
	find(page, func(e *html.Node) {
		if out == nil && strings.Contains(" "+attr(e, "class")+" ", " "+class+" ") {
			out = e
		}
	})
	return out
}

// TestTheCreationFormIsOneColumn asserts that the owner, the name and the
// button read top to bottom in one column. Set as a row, the name sat to
// the right of the owner list and the button under the owner alone, so the
// order a person fills the form in was not the order on the screen.
func TestTheCreationFormIsOneColumn(t *testing.T) {
	h := newHarness(t)
	page := doc(t, h.get("/new", h.signedIn("alice")).Body.String())
	fields := firstWithClass(page, "fields")
	if fields == nil {
		t.Fatal("the creation form has no field block")
	}
	if !strings.Contains(attr(fields, "class"), "stacked") {
		t.Errorf("the creation form's fields are set as a row: class %q", attr(fields, "class"))
	}
	if !strings.Contains(string(mustAsset(t, "app.css")), ".fields.stacked") {
		t.Error("the stylesheet has no one-column rule for a field block")
	}
}
