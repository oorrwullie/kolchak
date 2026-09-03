package projectmeta

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGitHubContributionTemplates(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, ".github")); err != nil {
		if os.IsNotExist(err) {
			t.Skip("GitHub metadata is not included in this source distribution")
		}
		t.Fatalf("stat GitHub metadata directory: %v", err)
	}

	tests := []struct {
		path     string
		contains []string
	}{
		{
			path: ".github/ISSUE_TEMPLATE/bug_report.yml",
			contains: []string{
				"name: Bug report",
				"description:",
				"expected-behavior",
				"actual-behavior",
				"reproduction",
				"kolchak-version",
				"go-version",
				"configuration",
				"logs",
				"secrets",
			},
		},
		{
			path: ".github/ISSUE_TEMPLATE/feature_request.yml",
			contains: []string{
				"name: Feature request",
				"problem",
				"proposed-outcome",
				"alternatives",
				"v0.1",
				"willing-to-contribute",
			},
		},
		{
			path: ".github/ISSUE_TEMPLATE/config.yml",
			contains: []string{
				"blank_issues_enabled: false",
				"https://github.com/oorrwullie/kolchak/security/advisories/new",
			},
		},
		{
			path: ".github/pull_request_template.md",
			contains: []string{
				"## Summary",
				"## Motivation",
				"## Testing",
				"## Compatibility and configuration",
				"## Checklist",
				"- [ ] The required `Test` check passes.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			text := string(content)
			for _, want := range tt.contains {
				if !strings.Contains(text, want) {
					t.Errorf("template missing %q", want)
				}
			}
		})
	}
}
