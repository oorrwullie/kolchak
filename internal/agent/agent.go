package agent

import "context"

// Request is the transport-neutral work an adapter sends to an agent.
type Request struct {
	Task string
}

// Input is retained as an alias while the engine migrates to Request.
type Input = Request

// Result is the transport-neutral outcome returned by an agent.
type Result struct {
	Events []Event
	Output string
}

// Event records an observable action emitted while an agent runs.
type Event struct {
	Type string
	Data map[string]any
}

// Agent runs one request through an implementation-specific adapter.
//
// Implementations must stop promptly when ctx is canceled. When cancellation
// wins the race with completion, the returned error must match ctx.Err() with
// errors.Is. Adapter failures use AdapterError; context cancellation does not.
type Agent interface {
	Run(context.Context, Request) (Result, error)
}
