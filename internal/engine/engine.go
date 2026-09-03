package engine

import (
	"context"
	"time"

	"github.com/oorrwullie/kolchak/internal/agent"
	"github.com/oorrwullie/kolchak/internal/faults"
	"github.com/oorrwullie/kolchak/internal/properties"
)

type Experiment struct {
	Agent      agent.Agent
	Input      agent.Input
	Faults     []faults.Fault
	Properties []properties.Property
	Seed       int64
}

type Outcome struct {
	Events     []agent.Event
	Violations []properties.Violation
	Duration   time.Duration
}

type Runner interface {
	Run(context.Context, Experiment) (Outcome, error)
}
