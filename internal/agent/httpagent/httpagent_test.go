package httpagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oorrwullie/kolchak/internal/agent"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewRejectsInvalidEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{name: "missing"},
		{name: "relative", endpoint: "/agent"},
		{name: "unsupported scheme", endpoint: "file:///tmp/agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.endpoint, nil); err == nil {
				t.Fatal("New() error = nil, want endpoint validation error")
			}
		})
	}
}

func TestRunExchangesJSONWithAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("request method = %q, want %q", r.Method, http.MethodPost)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", got, "application/json")
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q, want %q", got, "application/json")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body["task"]; got != "fix the failing test" {
			t.Fatalf("request task = %#v, want %q", got, "fix the failing test")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[{"type":"tool_call","data":{"name":"verify"}}],"output":"fixed"}`))
	}))
	defer server.Close()

	adapter, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := adapter.Run(context.Background(), agent.Request{Task: "fix the failing test"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "fixed" {
		t.Fatalf("result output = %q, want %q", result.Output, "fixed")
	}
	if len(result.Events) != 1 {
		t.Fatalf("result events = %d, want 1", len(result.Events))
	}
	if result.Events[0].Type != "tool_call" {
		t.Fatalf("event type = %q, want %q", result.Events[0].Type, "tool_call")
	}
	if got := result.Events[0].Data["name"]; got != "verify" {
		t.Fatalf("event data name = %#v, want %q", got, "verify")
	}
}

func TestRunRejectsNon2xxResponseWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("secret infrastructure detail"))
	}))
	defer server.Close()

	adapter, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.Run(context.Background(), agent.Request{})
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureRejected {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureRejected)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("Run() error = %q, want status code", err)
	}
	if strings.Contains(err.Error(), "secret infrastructure detail") {
		t.Errorf("Run() error = %q, must not contain response body", err)
	}
}

func TestRunClassifiesInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"events":`},
		{name: "unknown field", body: `{"events":[],"output":"","extra":true}`},
		{name: "second document", body: `{"events":[],"output":""}{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			adapter, err := New(server.URL, nil)
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

func TestRunClassifiesOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"events":[],"output":"` + strings.Repeat("x", maxResponseBytes) + `"}`))
	}))
	defer server.Close()

	adapter, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.Run(context.Background(), agent.Request{})
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureInvalidResponse {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureInvalidResponse)
	}
}

func TestRunClassifiesTransportFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	adapter, err := New("https://agent.example", client)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.Run(context.Background(), agent.Request{})
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureUnavailable {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureUnavailable)
	}
}
