package agent

import "context"

type Input struct{ Task string }

type Result struct {
	Events []Event
	Output string
}

type Event struct {
	Type string
	Data map[string]any
}

type Agent interface {
	Run(context.Context, Input) (Result, error)
}
