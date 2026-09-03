package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestAdapterErrorPreservesKindAndCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := fmt.Errorf("run request: %w", &AdapterError{
		Kind: FailureUnavailable,
		Err:  cause,
	})

	kind, ok := FailureKindOf(err)
	if !ok {
		t.Fatal("FailureKindOf() did not recognize AdapterError")
	}
	if kind != FailureUnavailable {
		t.Fatalf("FailureKindOf() = %q, want %q", kind, FailureUnavailable)
	}
	if !errors.Is(err, cause) {
		t.Fatal("AdapterError does not preserve its cause")
	}
}

func TestFailureKindOfDoesNotClassifyCancellation(t *testing.T) {
	err := fmt.Errorf("run request: %w", context.Canceled)

	if kind, ok := FailureKindOf(err); ok {
		t.Fatalf("FailureKindOf() = %q, true; want no adapter classification", kind)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation is not detectable with errors.Is")
	}
}

func TestAdapterErrorWithoutCause(t *testing.T) {
	err := (&AdapterError{Kind: FailureRejected}).Error()
	if want := "agent adapter rejected"; err != want {
		t.Fatalf("Error() = %q, want %q", err, want)
	}
}

func TestRequestSupportsHTTPAndCommandAdapters(t *testing.T) {
	var _ Agent = HTTPStub{}
	var _ Agent = CommandStub{}
}

type HTTPStub struct{}

func (HTTPStub) Run(context.Context, Request) (Result, error) {
	return Result{}, nil
}

type CommandStub struct{}

func (CommandStub) Run(context.Context, Request) (Result, error) {
	return Result{}, nil
}
