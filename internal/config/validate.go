package config

import (
	"fmt"
	"net/url"
	"strings"
)

type Problem struct {
	Path    string
	Message string
}

type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	problems := make([]string, len(e.Problems))
	for i, problem := range e.Problems {
		problems[i] = fmt.Sprintf("%s: %s", problem.Path, problem.Message)
	}
	return strings.Join(problems, "; ")
}

func (c Config) Validate() error {
	var validation ValidationError

	if c.Version != 1 {
		validation.add("version", "must be 1")
	}

	switch c.Agent.Type {
	case "":
		validation.add("agent.type", "is required")
	case "http":
		validateHTTPAgent(&validation, c.Agent)
	default:
		validation.add("agent.type", fmt.Sprintf("unsupported adapter %q; must be http", c.Agent.Type))
	}

	if len(c.Tests.Inputs) == 0 {
		validation.add("tests.inputs", "must contain at least one input")
	}
	for i, input := range c.Tests.Inputs {
		if strings.TrimSpace(input) == "" {
			validation.add(fmt.Sprintf("tests.inputs[%d]", i), "must not be empty")
		}
	}

	validateNames(&validation, "faults", c.Faults, []string{"timeout", "error", "malformed"})
	validateNames(&validation, "properties", c.Properties, []string{"verification_required"})

	if c.Runs.Confirm < 1 {
		validation.add("runs.confirm", "must be at least 1")
	}
	if c.Runs.Concurrency < 1 {
		validation.add("runs.concurrency", "must be at least 1")
	}

	if len(validation.Problems) > 0 {
		return &validation
	}
	return nil
}

func validateHTTPAgent(validation *ValidationError, agent Agent) {
	if strings.TrimSpace(agent.URL) == "" {
		validation.add("agent.url", "is required for the http adapter")
		return
	}

	parsed, err := url.Parse(agent.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		validation.add("agent.url", "must be an absolute http or https URL")
	}
}

func validateNames(validation *ValidationError, path string, values, supported []string) {
	if len(values) == 0 {
		validation.add(path, "must contain at least one value")
		return
	}

	allowed := make(map[string]struct{}, len(supported))
	for _, value := range supported {
		allowed[value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	for i, value := range values {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if _, ok := allowed[value]; !ok {
			validation.add(itemPath, fmt.Sprintf("unsupported value %q; must be one of %s", value, strings.Join(supported, ", ")))
		}
		if _, ok := seen[value]; ok {
			validation.add(itemPath, fmt.Sprintf("duplicates %q", value))
		}
		seen[value] = struct{}{}
	}
}

func (e *ValidationError) add(path, message string) {
	e.Problems = append(e.Problems, Problem{Path: path, Message: message})
}
