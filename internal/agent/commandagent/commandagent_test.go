package commandagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

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
	case "huge":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", 8<<20))
	case "exit":
		_, _ = io.WriteString(os.Stdout, `{"events":[],"output":"secret output"}`)
		_, _ = io.WriteString(os.Stderr, "private diagnostic")
		os.Exit(7)
	case "write-failure":
		if len(arguments) != 1 {
			_, _ = fmt.Fprintf(os.Stderr, "arguments = %#v, want one control address", arguments)
			os.Exit(2)
		}
		connection, err := net.Dial("tcp", arguments[0])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "connect for write failure: %v", err)
			os.Exit(2)
		}
		defer func() { _ = connection.Close() }()
		if _, err := connection.Read(make([]byte, 1)); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "wait for write failure: %v", err)
			os.Exit(2)
		}
		_ = os.Stdin.Close()
		os.Exit(7)
	case "block":
		if len(arguments) != 1 {
			_, _ = fmt.Fprintf(os.Stderr, "arguments = %#v, want one start address", arguments)
			os.Exit(2)
		}
		connection, err := net.Dial("tcp", arguments[0])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "signal process start: %v", err)
			os.Exit(2)
		}
		_ = connection.Close()
		for {
			time.Sleep(time.Hour)
		}
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

func TestWriteAllRejectsShortWrite(t *testing.T) {
	err := writeAll(shortWriter{}, []byte("request"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writeAll() error = %v, want io.ErrShortWrite", err)
	}
}

func TestUnavailableErrorClassifiesSetupFailure(t *testing.T) {
	err := unavailableError(errors.New("create command stdout"))
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureUnavailable {
		t.Fatalf("FailureKindOf(unavailableError()) = %q, %t; want %q, true", kind, ok, agent.FailureUnavailable)
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

func TestRunDrainsOversizedOutput(t *testing.T) {
	adapter, err := New(helperCommand("huge"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = adapter.Run(ctx, agent.Request{})
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureInvalidResponse {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureInvalidResponse)
	}
}

func TestRunClassifiesStartFailure(t *testing.T) {
	adapter, err := New([]string{"definitely-not-a-kolchak-command"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.Run(context.Background(), agent.Request{})
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureUnavailable {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureUnavailable)
	}
}

func TestRunPreservesAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter, err := New(helperCommand("success"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.Run(ctx, agent.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(Run() error, context.Canceled) = false; error = %v", err)
	}
	if kind, ok := agent.FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf(Run() error) = %q, true; want no adapter classification", kind)
	}
}

func TestRunRejectsNonzeroExit(t *testing.T) {
	adapter, err := New(helperCommand("exit"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.Run(context.Background(), agent.Request{})
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureRejected {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureRejected)
	}
	if !strings.Contains(err.Error(), "exit status 7") {
		t.Errorf("Run() error = %q, want exit status", err)
	}
	if !strings.Contains(err.Error(), "private diagnostic") {
		t.Errorf("Run() error = %q, want stderr diagnostic", err)
	}
	if strings.Contains(err.Error(), "secret output") {
		t.Errorf("Run() error = %q, must not contain stdout", err)
	}
}

func TestRunClassifiesRequestWriteFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for write failure: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	adapter, err := New(append(helperCommand("write-failure"), listener.Addr().String()))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	errs := make(chan error, 1)
	go func() {
		_, err := adapter.Run(context.Background(), agent.Request{Task: strings.Repeat("x", 8<<20)})
		errs <- err
	}()
	connection, err := listener.Accept()
	if err != nil {
		t.Fatalf("accept write-failure control connection: %v", err)
	}
	_, _ = connection.Write([]byte{1})
	_ = connection.Close()

	err = waitForRunError(t, errs)
	kind, ok := agent.FailureKindOf(err)
	if !ok || kind != agent.FailureUnavailable {
		t.Fatalf("FailureKindOf(Run() error) = %q, %t; want %q, true", kind, ok, agent.FailureUnavailable)
	}
}

func TestRejectedErrorBoundsStderrDiagnostic(t *testing.T) {
	diagnostic := strings.Repeat("z", maxOutputBytes+1)
	err := rejectedError(errors.New("exit status 7"), []byte(diagnostic))
	if got := strings.Count(err.Error(), "z"); got != maxOutputBytes {
		t.Errorf("rejectedError() diagnostic bytes = %d, want %d", got, maxOutputBytes)
	}
}

func TestRunPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errs := startBlockedCommand(t, ctx)
	cancel()
	err := waitForRunError(t, errs)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(Run() error, context.Canceled) = false; error = %v", err)
	}
	if kind, ok := agent.FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf(Run() error) = %q, true; want no adapter classification", kind)
	}
}

func TestRunPreservesDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	errs := startBlockedCommand(t, ctx)
	err := waitForRunError(t, errs)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(Run() error, context.DeadlineExceeded) = false; error = %v", err)
	}
	if kind, ok := agent.FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf(Run() error) = %q, true; want no adapter classification", kind)
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

func startBlockedCommand(t *testing.T, ctx context.Context) <-chan error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for command start: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	adapter, err := New(append(helperCommand("block"), listener.Addr().String()))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	errs := make(chan error, 1)
	go func() {
		_, err := adapter.Run(ctx, agent.Request{})
		errs <- err
	}()

	started := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			_ = connection.Close()
		}
		close(started)
	}()
	select {
	case <-started:
	case err := <-errs:
		t.Fatalf("Run() returned before command started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("command did not start")
	}

	return errs
}

func waitForRunError(t *testing.T, errs <-chan error) error {
	t.Helper()
	select {
	case err := <-errs:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return")
		return nil
	}
}

type shortWriter struct{}

func (shortWriter) Write(body []byte) (int, error) {
	return len(body) - 1, nil
}
