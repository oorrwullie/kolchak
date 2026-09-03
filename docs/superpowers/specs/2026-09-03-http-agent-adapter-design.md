# HTTP Agent Adapter Design

## Purpose

`KOLCHAK-7` adds Kolchak's first concrete agent adapter. The adapter sends a
transport-neutral `agent.Request` to a configured HTTP endpoint and converts
the endpoint's response into `agent.Result` without exposing HTTP details to
the experiment engine.

## Decision

Use a small synchronous JSON protocol over HTTP `POST`:

```json
{"task":"Fix the failing test"}
```

A successful endpoint returns:

```json
{
  "events": [
    {"type":"tool_call","data":{"name":"verify"}}
  ],
  "output":"The test is fixed."
}
```

The request and response map directly to `agent.Request`, `agent.Event`, and
`agent.Result`. The shared agent package owns those semantic types; the HTTP
adapter owns JSON encoding, status handling, response-size limits, and HTTP
client behavior.

This v0.1 protocol deliberately excludes streaming, authentication helpers,
retries, custom headers, and protocol-version envelopes. Callers may supply an
`http.Client` whose transport implements environment-specific authentication.

## Alternatives Considered

### Versioned envelope

A top-level protocol version and metadata envelope would make future wire
evolution explicit, but it adds fields that no v0.1 consumer needs. Kolchak can
introduce a new content type or endpoint contract if compatibility is needed
before a stable release.

### Streaming events

Server-sent events or newline-delimited JSON would expose progress before the
agent finishes, but it substantially complicates cancellation, partial-result
semantics, and tests. The experiment engine currently consumes completed
results, so synchronous JSON is the smaller boundary.

### Adapter-specific request and response models

Separate HTTP DTOs would isolate wire types but duplicate the entire current
contract. Direct mapping is appropriate while the shapes are identical. If the
wire protocol diverges later, private DTOs can be introduced inside the HTTP
adapter without changing the `agent.Agent` interface.

## Package and API

Add `internal/agent/httpagent` with a concrete adapter:

```go
type Adapter struct { /* private fields */ }

func New(endpoint string, client *http.Client) (*Adapter, error)
func (a *Adapter) Run(ctx context.Context, req agent.Request) (agent.Result, error)
```

`New` validates that the endpoint is an absolute `http` or `https` URL. A nil
client uses `http.DefaultClient`. The adapter retains the supplied client but
does not mutate it.

Each call creates an HTTP request with `http.NewRequestWithContext`, sets
`Content-Type: application/json` and `Accept: application/json`, and sends it
through the configured client.

## Response Boundaries

The adapter reads at most 1 MiB of response data. A response beyond that limit
is classified as invalid rather than being partially decoded. Successful
responses must contain one JSON document with no trailing non-whitespace data.
Unknown JSON fields are rejected so protocol mistakes surface immediately.

An empty `events` list and empty `output` are valid transport results. Whether
they satisfy a behavioral property belongs to the experiment engine and
property evaluators, not the HTTP adapter.

## Error Mapping

- Local request construction failure: returned with operation context rather
  than a remote adapter classification.
- Client transport failure before an HTTP response: `FailureUnavailable`.
- Context cancellation or deadline: return an error matching `ctx.Err()` via
  `errors.Is`, without wrapping it in `AdapterError`.
- Any non-2xx response: `FailureRejected`. The error includes the status code
  but not the response body, which may contain agent or infrastructure data.
- Oversized, malformed, unknown-field, or multi-document JSON response:
  `FailureInvalidResponse`.

The adapter does not retry. Retrying agent execution can duplicate side effects
and must be an explicit experiment-engine policy if introduced later.

## Cancellation

The request context owns the lifetime of the HTTP operation. Canceling it must
interrupt connection establishment, request transmission, and response reads.
When cancellation wins, `Run` returns an error for which
`errors.Is(err, ctx.Err())` is true.

## Testing

Use `httptest.Server` and real `http.Client` behavior. Tests cover:

- successful request method, headers, JSON body, and normalized response;
- non-2xx classification and status-code diagnostics without body disclosure;
- malformed and unknown-field response classification;
- oversized response rejection;
- client transport failure classification;
- deadline expiration and explicit cancellation with `errors.Is` checks;
- constructor rejection of missing, relative, and unsupported-scheme URLs.

Tests use only loopback HTTP servers and require no external network access.

## Compatibility and Scope

The adapter is internal and the project is pre-v0.1, but the JSON shape is the
first executable agent protocol. This story keeps that shape intentionally
small. Command-adapter configuration, shared adapter contract suites, event
model expansion, retries, authentication UX, and streaming remain separate
stories.
