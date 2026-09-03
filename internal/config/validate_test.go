package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default().Validate() = %v", err)
	}
}

func TestValidateReportsEveryInvalidPath(t *testing.T) {
	cfg := Config{
		Version:    2,
		Agent:      Agent{Type: "command", URL: "not a URL"},
		Tests:      Tests{Inputs: []string{""}},
		Faults:     []string{"network", "network"},
		Properties: []string{"helpful", "helpful"},
		Runs:       Runs{},
	}

	err := cfg.Validate()
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Validate() error = %T %v, want *ValidationError", err, err)
	}

	got := make([]string, len(validation.Problems))
	for i, problem := range validation.Problems {
		got[i] = problem.Path
	}
	want := []string{
		"version",
		"agent.command",
		"tests.inputs[0]",
		"faults[0]",
		"faults[1]",
		"faults[1]",
		"properties[0]",
		"properties[1]",
		"properties[1]",
		"runs.confirm",
		"runs.concurrency",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problem paths = %#v, want %#v", got, want)
	}
}

func TestValidateHTTPAgent(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "missing", want: "is required"},
		{name: "relative", url: "/agent", want: "absolute http or https URL"},
		{name: "unsupported scheme", url: "file:///tmp/agent", want: "absolute http or https URL"},
		{name: "valid http", url: "http://localhost:8080/agent"},
		{name: "valid https", url: "https://example.com/agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Agent.URL = tt.url

			err := cfg.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "agent.url:") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want agent.url error containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateCommandAgent(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		want    string
	}{
		{name: "missing", want: "is required for the command adapter"},
		{name: "blank executable", command: []string{" "}, want: "agent.command[0]: must not be empty"},
		{name: "blank argument", command: []string{"agent", " "}, want: "agent.command[1]: must not be empty"},
		{name: "valid", command: []string{"agent", "--stdio"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Agent = Agent{Type: "command", Command: tt.command}

			err := cfg.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadValidatesConfiguration(t *testing.T) {
	path := writeConfig(t, "agent:\n  type: unsupported\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "validate config") || !strings.Contains(err.Error(), "agent.type") {
		t.Fatalf("Load() error = %q, want validation context and field path", err)
	}
}
