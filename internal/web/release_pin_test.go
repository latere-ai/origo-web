// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package web

import (
	"os"
	"regexp"
	"testing"

	"github.com/goccy/go-yaml"
)

// The production overlay names the image tag the cluster runs. The release
// workflow does not write it, so a tag cut without bumping it leaves the
// repository describing an older release than the one that shipped, and
// anyone applying the overlay rolls production backwards. It drifted twice
// before this test existed: v0.4.2 while the cluster ran v0.5.0, and v0.5.2
// while v0.5.3 was published.
func TestProductionOverlayNamesTheNewestRelease(t *testing.T) {
	body, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^## (v\d+\.\d+\.\d+) - \d{4}-\d{2}-\d{2}$`).FindSubmatch(body)
	if m == nil {
		t.Fatal("CHANGELOG.md names no release")
	}
	want := string(m[1])

	overlay, err := os.ReadFile("../../deploy/prod/kustomization.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Images []struct {
			Name   string `yaml:"name"`
			NewTag string `yaml:"newTag"`
			Digest string `yaml:"digest"`
		} `yaml:"images"`
	}
	if err := yaml.Unmarshal(overlay, &config); err != nil {
		t.Fatal(err)
	}
	for _, image := range config.Images {
		if image.Name != "ghcr.io/latere-ai/origoweb" {
			continue
		}
		if image.NewTag != want {
			t.Errorf("deploy/prod pins %s, the newest release is %s", image.NewTag, want)
		}
		if image.Digest != "" {
			t.Errorf("digest %s overrides the release tag", image.Digest)
		}
		return
	}
	t.Fatal("deploy/prod/kustomization.yaml does not pin the origoweb image")
}
