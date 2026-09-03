package faults

import "context"

type ToolCall struct {
	Name      string
	Arguments map[string]any
}

type Outcome struct {
	Result any
	Err    error
}

type Fault interface {
	Name() string
	Apply(context.Context, ToolCall) Outcome
}
