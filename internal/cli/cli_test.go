package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInit(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer

	if err := Run([]string{"init", dir}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Initialized Kolchak") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
	for _, name := range []string{"kolchak.yaml", ".kolchak/runs", ".kolchak/failures", ".kolchak/cases"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s: %v", name, err)
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := Run([]string{"test"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected an error")
	}
}
