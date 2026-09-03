package httpagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oorrwullie/kolchak/internal/agent"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type readSignalBody struct {
	io.ReadCloser
	readStarted chan<- struct{}
}

const testWaitTimeout = 2 * time.Second

func (b *readSignalBody) Read(p []byte) (int, error) {
	select {
	case b.readStarted <- struct{}{}:
	default:
	}
	return b.ReadCloser.Read(p)
}

func waitForStage(t *testing.T, stage string, signal <-chan struct{}, errs <-chan error) {
	t.Helper()
	select {
	case <-signal:
	case err := <-errs:
		t.Fatalf("Run() returned before %s: %v", stage, err)
	case <-time.After(testWaitTimeout):
		t.Fatalf("timed out waiting for %s", stage)
	}
}

func waitForRunError(t *testing.T, errs <-chan error) error {
	t.Helper()
	select {
	case err := <-errs:
		return err
	case <-time.After(testWaitTimeout):
		t.Fatal("timed out waiting for Run() to return")
		return nil
	}
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

func TestRunRejectsNon2xxResponseWithoutDisclosingBody(t *testing.T) {
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

func TestRunRejectsRedirectWithoutReplayingPost(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		statusText string
	}{
		{name: "temporary", statusCode: http.StatusTemporaryRedirect, statusText: "307"},
		{name: "permanent", statusCode: http.StatusPermanentRedirect, statusText: "308"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectTargetRequests := make(chan *http.Request, 1)
			redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectTargetRequests <- r
				w.WriteHeader(http.StatusNoContent)
			}))
			defer redirectTarget.Close()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", redirectTarget.URL)
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			adapter, err := New(server.URL, nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			_, err = adapter.Run(context.Background(), agent.Request{Task: "do not replay"})
			kind, ok := agent.FailureKindOf(err)
			if !ok || kind != agent.FailureRejected {
				t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureRejected)
			}
			if !strings.Contains(err.Error(), tt.statusText) {
				t.Errorf("Run() error = %q, want original redirect status", err)
			}
			select {
			case request := <-redirectTargetRequests:
				t.Fatalf("redirect target received replayed %s request", request.Method)
			case <-time.After(100 * time.Millisecond):
			}
		})
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

func TestRunPreservesCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	adapter, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func() {
		_, err := adapter.Run(ctx, agent.Request{})
		errs <- err
	}()
	waitForStage(t, "handler start", started, errs)
	cancel()
	err = waitForRunError(t, errs)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(Run() error, context.Canceled) = false; error = %v", err)
	}
	if kind, ok := agent.FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf(Run() error) = %q, true; want no adapter classification", kind)
	}
}

func TestRunPreservesCancellationDuringResponseBodyRead(t *testing.T) {
	headersFlushed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(headersFlushed)
		<-r.Context().Done()
	}))
	defer server.Close()
	bodyReadStarted := make(chan struct{}, 1)
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		response, err := http.DefaultTransport.RoundTrip(request)
		if err == nil {
			response.Body = &readSignalBody{ReadCloser: response.Body, readStarted: bodyReadStarted}
		}
		return response, err
	})}

	adapter, err := New(server.URL, client)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func() {
		_, err := adapter.Run(ctx, agent.Request{})
		errs <- err
	}()
	waitForStage(t, "response headers", headersFlushed, errs)
	waitForStage(t, "response body read", bodyReadStarted, errs)
	cancel()
	err = waitForRunError(t, errs)
	if !errors.Is(err, ctx.Err()) {
		t.Fatalf("errors.Is(Run() error, ctx.Err()) = false; error = %v", err)
	}
	if kind, ok := agent.FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf(Run() error) = %q, true; want no adapter classification", kind)
	}
}

func TestRunPreservesDeadline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	adapter, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errs := make(chan error, 1)
	go func() {
		_, err := adapter.Run(ctx, agent.Request{})
		errs <- err
	}()
	select {
	case <-started:
	case err := <-errs:
		t.Fatalf("Run() returned before handler started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start before timeout")
	}
	select {
	case err = <-errs:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return before timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(Run() error, context.DeadlineExceeded) = false; error = %v", err)
	}
	if kind, ok := agent.FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf(Run() error) = %q, true; want no adapter classification", kind)
	}
}
