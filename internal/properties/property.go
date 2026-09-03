package properties

import "github.com/oorrwullie/kolchak/internal/agent"

type Violation struct {
	Property string
	Message  string
}

type Property interface {
	Name() string
	Evaluate([]agent.Event) []Violation
}
