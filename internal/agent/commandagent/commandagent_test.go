package commandagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/oorrwullie/kolchak/internal/agent"
)

func TestHelperProcess(t *testing.T) {
	mode, arguments, ok := helperMode(os.Args)
	if !ok {
		return
	}

	switch mode {
	case "success":
		if len(arguments) != 1 || arguments[0] != "--stdio" {
			_, _ = fmt.Fprintf(os.Stderr, "arguments = %#v, want [--stdio]", arguments)
			os.Exit(2)
		}
		var request agent.Request
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "decode request: %v", err)
			os.Exit(2)
		}
		if request.Task != "Fix the failing test" {
			_, _ = fmt.Fprintf(os.Stderr, "task = %q, want %q", request.Task, "Fix the failing test")
			os.Exit(2)
		}
		_, _ = io.WriteString(os.Stdout, `{"events":[{"type":"tool_call","data":{"name":"verify"}}],"output":"fixed"}`)
	case "malformed":
		_, _ = io.WriteString(os.Stdout, `{"events":`)
	case "unknown":
		_, _ = io.WriteString(os.Stdout, `{"events":[],"output":"","extra":true}`)
	case "multiple":
		_, _ = io.WriteString(os.Stdout, `{"events":[],"output":""}{}`)
	case "large":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", maxOutputBytes+1))
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown helper mode %q", mode)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestNewRejectsInvalidCommand(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "nil"},
		{name: "empty", argv: []string{}},
		{name: "blank executable", argv: []string{" "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.argv); err == nil {
				t.Fatalf("New(%#v) error = nil, want validation error", tt.argv)
			}
		})
	}
}

func TestRunExchangesJSONWithCommand(t *testing.T) {
	adapter, err := New(append(helperCommand("success"), "--stdio"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := adapter.Run(context.Background(), agent.Request{Task: "Fix the failing test"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "fixed" {
		t.Errorf("Run().Output = %q, want %q", result.Output, "fixed")
	}
	if len(result.Events) != 1 {
		t.Fatalf("Run().Events = %#v, want one event", result.Events)
	}
	if result.Events[0].Type != "tool_call" {
		t.Errorf("Run().Events[0].Type = %q, want %q", result.Events[0].Type, "tool_call")
	}
	if got := result.Events[0].Data["name"]; got != "verify" {
		t.Errorf("Run().Events[0].Data[name] = %#v, want %q", got, "verify")
	}
}

func TestRunClassifiesInvalidOutput(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "malformed", mode: "malformed"},
		{name: "unknown field", mode: "unknown"},
		{name: "multiple documents", mode: "multiple"},
		{name: "oversized", mode: "large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := New(helperCommand(tt.mode))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			_, err = adapter.Run(context.Background(), agent.Request{})
			kind, ok := agent.FailureKindOf(err)
			if !ok || kind != agent.FailureInvalidResponse {
				t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureInvalidResponse)
			}
		})
	}
}

func helperCommand(mode string) []string {
	return []string{os.Args[0], "-test.run=TestHelperProcess", "--", mode}
}

func helperMode(arguments []string) (string, []string, bool) {
	for i, argument := range arguments {
		if argument == "--" && i+1 < len(arguments) {
			return arguments[i+1], arguments[i+2:], true
		}
	}
	return "", nil, false
}
