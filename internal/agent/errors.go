package agent

import (
	"errors"
	"fmt"
)

// FailureKind classifies adapter failures without exposing HTTP or subprocess
// implementation details to the experiment engine.
type FailureKind string

const (
	// FailureUnavailable means the adapter could not reach or start the agent.
	FailureUnavailable FailureKind = "unavailable"
	// FailureRejected means the agent ran but its transport reported failure.
	FailureRejected FailureKind = "rejected"
	// FailureInvalidResponse means the agent returned an undecodable result.
	FailureInvalidResponse FailureKind = "invalid_response"
)

// AdapterError is a classified failure at the agent adapter boundary.
type AdapterError struct {
	Kind FailureKind
	Err  error
}

func (e *AdapterError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return fmt.Sprintf("agent adapter %s", e.Kind)
	}
	return fmt.Sprintf("agent adapter %s: %v", e.Kind, e.Err)
}

func (e *AdapterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// FailureKindOf reports the normalized kind of an adapter failure.
func FailureKindOf(err error) (FailureKind, bool) {
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr == nil {
		return "", false
	}
	return adapterErr.Kind, true
}
