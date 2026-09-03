// Package httpagent adapts the agent contract to a synchronous JSON-over-HTTP endpoint.
package httpagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		return agent.Result{}, fmt.Errorf("send HTTP request: %w", err)
	}
	defer response.Body.Close()

	var result agent.Result
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return agent.Result{}, fmt.Errorf("decode HTTP response: %w", err)
	}
	return result, nil
}
