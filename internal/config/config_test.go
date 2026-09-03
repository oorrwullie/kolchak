package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	path := writeConfig(t, `version: 1
agent:
  type: http
  url: http://localhost:8080/agent
tests:
  inputs:
    - Fix the failing test
faults:
  - timeout
properties:
  - verification_required
runs:
  confirm: 20
  concurrency: 2
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Agent.Type != "http" || cfg.Agent.URL != "http://localhost:8080/agent" {
		t.Errorf("Agent = %#v", cfg.Agent)
	}
	if len(cfg.Tests.Inputs) != 1 || cfg.Tests.Inputs[0] != "Fix the failing test" {
		t.Errorf("Tests.Inputs = %#v", cfg.Tests.Inputs)
	}
	if len(cfg.Faults) != 1 || cfg.Faults[0] != "timeout" {
		t.Errorf("Faults = %#v", cfg.Faults)
	}
	if len(cfg.Properties) != 1 || cfg.Properties[0] != "verification_required" {
		t.Errorf("Properties = %#v", cfg.Properties)
	}
	if cfg.Runs.Confirm != 20 || cfg.Runs.Concurrency != 2 {
		t.Errorf("Runs = %#v", cfg.Runs)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := writeConfig(t, "agent:\n  url: http://localhost:8080/agent\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Agent.Type != "http" {
		t.Errorf("Agent.Type = %q, want http", cfg.Agent.Type)
	}
	if cfg.Runs.Confirm != 10 || cfg.Runs.Concurrency != 4 {
		t.Errorf("Runs = %#v, want defaults", cfg.Runs)
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	path := writeConfig(t, "agent: [\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "decode config") || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want path and decode context", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := writeConfig(t, "version: 1\nunknown: true\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("error = %q, want unknown field context", err)
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	path := writeConfig(t, "version: 1\n---\nversion: 1\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("error = %q, want multiple document context", err)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kolchak.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
