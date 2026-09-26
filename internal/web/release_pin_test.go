// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"os"
	"regexp"
	"testing"
)

// The install page names the release twice: the image `docker run` starts,
// and the tag its example overlay pins. `lateregate release` rewrites both
// in the commit that cuts the tag, so a version here that is not the newest
// release in the changelog is a stamp that missed, and a reader copying the
// page would run an older release than the one published.
func TestTheInstallPageNamesTheNewestRelease(t *testing.T) {
	body, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^## (v\d+\.\d+\.\d+) - \d{4}-\d{2}-\d{2}$`).FindSubmatch(body)
	if m == nil {
		t.Fatal("CHANGELOG.md names no release")
	}
	want := string(m[1])

	page, err := os.ReadFile("../../docs/install.md")
	if err != nil {
		t.Fatal(err)
	}
	for what, re := range map[string]*regexp.Regexp{
		"the image":           regexp.MustCompile(`ghcr\.io/latere-ai/origoweb:(v\d+\.\d+\.\d+)`),
		"the example overlay": regexp.MustCompile(`newTag: (v\d+\.\d+\.\d+)`),
	} {
		found := re.FindAllSubmatch(page, -1)
		if len(found) != 1 {
			t.Errorf("docs/install.md names %s's release %d times, want exactly one", what, len(found))
			continue
		}
		if got := string(found[0][1]); got != want {
			t.Errorf("docs/install.md names %s at %s, the newest release is %s", what, got, want)
		}
	}
}
