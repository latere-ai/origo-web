// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: MIT

package ci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestPublicVerificationUsesHostedRunners(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	body, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../.github/workflows/verify.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			RunsOn string            `yaml:"runs-on"`
			Uses   string            `yaml:"uses"`
			With   map[string]string `yaml:"with"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Jobs) == 0 {
		t.Fatal("verification has no jobs")
	}
	for name, job := range workflow.Jobs {
		if job.Uses == "" {
			if job.RunsOn != "ubuntu-latest" {
				t.Errorf("%s requests %q; the org runner excludes public repositories", name, job.RunsOn)
			}
			continue
		}
		if job.With["runs_on"] != "ubuntu-latest" {
			t.Errorf("%s routes shared jobs to a runner group that excludes this repository", name)
		}
		var matrix []string
		if err := json.Unmarshal([]byte(job.With["test_os"]), &matrix); err != nil || len(matrix) != 1 || matrix[0] != "ubuntu-latest" {
			t.Errorf("%s must run its Linux test matrix on hosted runners", name)
		}
	}
}
