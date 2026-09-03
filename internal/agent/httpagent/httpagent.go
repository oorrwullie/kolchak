// Package httpagent adapts the agent contract to a synchronous JSON-over-HTTP endpoint.
package httpagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/oorrwullie/kolchak/internal/agent"
)

const maxResponseBytes = 1 << 20

// Adapter sends agent requests to an HTTP endpoint.
type Adapter struct {
	endpoint string
	client   *http.Client
}

var _ agent.Agent = (*Adapter)(nil)

// New creates an Adapter for an absolute HTTP or HTTPS endpoint.
func New(endpoint string, client *http.Client) (*Adapter, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("HTTP agent endpoint must be an absolute http or https URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{endpoint: endpoint, client: client}, nil
}

// Run sends req to the configured endpoint and returns its JSON result.
func (a *Adapter) Run(ctx context.Context, req agent.Request) (agent.Result, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return agent.Result{}, fmt.Errorf("marshal agent request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return agent.Result{}, fmt.Errorf("create HTTP request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	response, err := a.client.Do(httpRequest)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return agent.Result{}, err
		}
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureUnavailable,
			Err:  err,
		}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureRejected,
			Err:  fmt.Errorf("HTTP agent returned status %d", response.StatusCode),
		}
	}

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureInvalidResponse,
			Err:  fmt.Errorf("read HTTP response: %w", err),
		}
	}
	if len(responseBody) > maxResponseBytes {
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureInvalidResponse,
			Err:  errors.New("HTTP agent response exceeds maximum size"),
		}
	}

	var result agent.Result
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureInvalidResponse,
			Err:  fmt.Errorf("decode HTTP response: %w", err),
		}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("HTTP response contains multiple JSON documents")
		}
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureInvalidResponse,
			Err:  fmt.Errorf("decode HTTP response: %w", err),
		}
	}
	return result, nil
}
