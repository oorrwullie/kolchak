package project

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oorrwullie/kolchak/internal/config"
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

func TestInitConfigRoundTrip(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	secondDir := filepath.Join(t.TempDir(), "second")
	for _, dir := range []string{firstDir, secondDir} {
		if err := Init(dir); err != nil {
			t.Fatalf("Init(%q): %v", dir, err)
		}
	}

	firstPath := filepath.Join(firstDir, ConfigName)
	cfg, err := config.Load(firstPath)
	if err != nil {
		t.Fatalf("load generated config: %v", err)
	}
	if want := config.Default(); !reflect.DeepEqual(cfg, want) {
		t.Fatalf("generated config = %#v, want %#v", cfg, want)
	}

	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(secondDir, ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("generated configuration is not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
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
