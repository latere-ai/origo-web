// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"os"
	"strings"
	"testing"
)

// TestNoScreenExplainsItself bans the phrases the interface used to reach for
// when a sentence was written for effect rather than to say what to do. Each
// one is a tell: a screen that names "whoever administers this installation"
// instead of "your administrator", or that tells a reader why a control is
// absent instead of leaving it absent. The list is short on purpose; the
// help-line ceiling and the "X, not Y" ban catch the rest. It reads the
// screens as a reader receives them and the sources they come from: the
// templates, this package's Go strings, the README and the release notes.
func TestNoScreenExplainsItself(t *testing.T) {
	banned := []string{
		"whoever administers", "the whole point", "worse than none",
		"nothing is hidden", "in the spirit of", "nobody chose",
	}
	h := newHarness(t)
	for name, rec := range h.everyPage() {
		body := strings.ToLower(text(elements(doc(t, rec.Body.String()), "body")[0]))
		for _, phrase := range banned {
			if strings.Contains(body, phrase) {
				t.Errorf("%s says %q", name, phrase)
			}
		}
	}
	for _, root := range []string{"templates", ".", "../../CHANGELOG.md", "../../README.md"} {
		for _, path := range filesUnder(t, root) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				for _, phrase := range banned {
					if strings.Contains(strings.ToLower(line), phrase) {
						t.Errorf("%s:%d says %q", path, i+1, phrase)
					}
				}
			}
		}
	}
}
