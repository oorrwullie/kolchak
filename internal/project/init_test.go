package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent")
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Fatalf("unexpected config: %s", data)
	}
	for _, name := range []string{"runs", "failures", "cases"} {
		info, err := os.Stat(filepath.Join(dir, ".kolchak", name))
		if err != nil || !info.IsDir() {
			t.Errorf("expected directory %s", name)
		}
	}
}

func TestInitDoesNotOverwriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	if err := os.WriteFile(path, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir); err == nil {
		t.Fatal("expected an error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "mine\n" {
		t.Fatalf("config was overwritten: %q", data)
	}
}
