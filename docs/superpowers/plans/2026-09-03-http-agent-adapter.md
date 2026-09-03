# HTTP Agent Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the synchronous JSON HTTP adapter described by `KOLCHAK-7`.

**Architecture:** Add `internal/agent/httpagent` with a concrete `agent.Agent`. The package owns endpoint validation, HTTP request construction, bounded response reading, strict JSON decoding, and conversion of HTTP failures into the existing transport-neutral error taxonomy.

**Tech Stack:** Go 1.26+, `net/http`, `encoding/json`, `httptest`, and the existing `internal/agent` contract.

**Spec:** `docs/superpowers/specs/2026-09-03-http-agent-adapter-design.md`

## Global Constraints

- POST JSON request `{"task":"..."}` and accept `{"events":[...],"output":"..."}`.
- Accept only absolute `http` and `https` URLs.
- A nil client means `http.DefaultClient`; never mutate a supplied client.
- Set `Content-Type` and `Accept` to `application/json`.
- Limit responses to 1 MiB and reject unknown fields or extra documents.
- Do not disclose non-2xx response bodies, retry, stream, or add authentication UX.
- Cancellation must match `ctx.Err()` via `errors.Is`, without `AdapterError` classification.
- Tests use only loopback servers or in-memory transports.
- The branch and PR contain only `KOLCHAK-7` work.

---

### Task 1: Constructor and successful JSON exchange

**Files:**
- Modify: `internal/agent/agent.go`
- Create: `internal/agent/httpagent/httpagent.go`
- Create: `internal/agent/httpagent/httpagent_test.go`

**Interfaces:**
- Consumes: `agent.Request`, `agent.Result`, `agent.Event`, and `agent.Agent`.
- Produces: `New(endpoint string, client *http.Client) (*Adapter, error)` and `(*Adapter).Run(context.Context, agent.Request) (agent.Result, error)`.

- [ ] **Step 1: Write failing constructor tests**

Add this table to `httpagent_test.go`:

```go
func TestNewRejectsInvalidEndpoint(t *testing.T) {
    tests := []struct {
        name     string
        endpoint string
    }{
        {name: "missing"},
        {name: "relative", endpoint: "/agent"},
        {name: "unsupported scheme", endpoint: "file:///tmp/agent"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if _, err := New(tt.endpoint, nil); err == nil {
                t.Fatal("New() error = nil, want endpoint validation error")
            }
        })
    }
}
```

- [ ] **Step 2: Write the failing successful-exchange test**

Use `httptest.NewServer` to assert POST, both JSON headers, and decoded body key `task`. Return literal JSON containing one `tool_call` event and output `fixed`; assert the resulting `agent.Result` contains those literal values.

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./internal/agent/httpagent -count=1`

Expected: FAIL because the package and `New` do not exist.

- [ ] **Step 4: Add explicit JSON tags to shared types**

Modify `internal/agent/agent.go`:

```go
type Request struct {
    Task string `json:"task"`
}

type Result struct {
    Events []Event `json:"events"`
    Output string  `json:"output"`
}

type Event struct {
    Type string         `json:"type"`
    Data map[string]any `json:"data"`
}
```

- [ ] **Step 5: Implement the constructor and success path**

Create `httpagent.go` with:

```go
const maxResponseBytes = 1 << 20

type Adapter struct {
    endpoint string
    client   *http.Client
}

var _ agent.Agent = (*Adapter)(nil)

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
```

Implement `Run` by marshaling `agent.Request`, calling `http.NewRequestWithContext`, setting the two headers, sending with `a.client.Do`, and decoding one `agent.Result`. This task intentionally handles only the green success path; Task 2 replaces decoding with the strict bounded implementation.

- [ ] **Step 6: Run package tests and verify GREEN**

Run: `go test ./internal/agent/httpagent ./internal/agent -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/agent.go internal/agent/httpagent
git commit -m "feat(httpagent): add successful JSON exchange"
```

---

### Task 2: Bound and classify failures

**Files:**
- Modify: `internal/agent/httpagent/httpagent.go`
- Modify: `internal/agent/httpagent/httpagent_test.go`

**Interfaces:**
- Consumes: `agent.AdapterError` and all three `agent.FailureKind` constants.
- Produces: strict bounded decoding and normalized failure mapping from `Adapter.Run`.

- [ ] **Step 1: Write failing non-2xx test**

Serve status 502 with body `secret infrastructure detail`. Assert `FailureKindOf(err)` is `FailureRejected`, the error contains `502`, and it does not contain the response body.

- [ ] **Step 2: Write failing invalid-response table test**

Use these literal bodies and assert `FailureInvalidResponse` for each:

```go
tests := []struct {
    name string
    body string
}{
    {name: "malformed", body: `{"events":`},
    {name: "unknown field", body: `{"events":[],"output":"","extra":true}`},
    {name: "second document", body: `{"events":[],"output":""}{}`},
}
```

- [ ] **Step 3: Write failing size and transport tests**

Serve a JSON body larger than `maxResponseBytes` and assert `FailureInvalidResponse`. Supply a client whose `RoundTripper` returns `errors.New("dial failed")` and assert `FailureUnavailable`.

- [ ] **Step 4: Run failure tests and verify RED**

Run: `go test ./internal/agent/httpagent -run 'TestRun(Classifies|Rejects)' -count=1`

Expected: FAIL because the success-only path neither limits nor classifies failures.

- [ ] **Step 5: Implement status and transport mapping**

After `client.Do`, use:

```go
if err != nil {
    return agent.Result{}, &agent.AdapterError{
        Kind: agent.FailureUnavailable,
        Err:  err,
    }
}
if response.StatusCode < 200 || response.StatusCode >= 300 {
    return agent.Result{}, &agent.AdapterError{
        Kind: agent.FailureRejected,
        Err:  fmt.Errorf("HTTP agent returned status %d", response.StatusCode),
    }
}
```

- [ ] **Step 6: Implement bounded strict decoding**

Read with `io.LimitReader(response.Body, maxResponseBytes+1)`. Reject `len(body) > maxResponseBytes`. Decode from `bytes.NewReader(body)` using `DisallowUnknownFields`, then perform a second decode and require `io.EOF`. Wrap read, limit, and decode errors in `AdapterError{Kind: FailureInvalidResponse}`.

- [ ] **Step 7: Run package tests and verify GREEN**

Run: `go test ./internal/agent/httpagent -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/agent/httpagent
git commit -m "feat(httpagent): classify bounded responses"
```

---

### Task 3: Preserve cancellation and finish verification

**Files:**
- Modify: `internal/agent/httpagent/httpagent.go`
- Modify: `internal/agent/httpagent/httpagent_test.go`

**Interfaces:**
- Consumes: `Adapter.Run` and the shared `agent.Agent` cancellation contract.
- Produces: direct evidence for `context.Canceled` and `context.DeadlineExceeded` identity.

- [ ] **Step 1: Write the cancellation test**

Use a loopback handler that closes a `started` channel and blocks on `r.Context().Done()`. Run the adapter in a goroutine, wait for `started`, cancel the context, and assert `errors.Is(err, context.Canceled)` and that `FailureKindOf(err)` returns false.

- [ ] **Step 2: Write the deadline test**

Use the same blocking handler with `context.WithTimeout`. Assert `errors.Is(err, context.DeadlineExceeded)` and no adapter classification.

- [ ] **Step 3: Run focused cancellation tests**

Run: `go test ./internal/agent/httpagent -run 'TestRunPreserves(Cancellation|Deadline)' -count=1`

Expected: FAIL because Task 2 classifies every `client.Do` error as `FailureUnavailable`.

- [ ] **Step 4: Correct context precedence**

The `client.Do` error branch must return `ctx.Err()` before constructing `AdapterError`. Make only that correction, then re-run Step 3.

- [ ] **Step 5: Run the full local quality gate**

Run each command and require exit zero:

```bash
gofmt -w internal/agent/agent.go internal/agent/httpagent/httpagent.go internal/agent/httpagent/httpagent_test.go
golangci-lint fmt --diff
golangci-lint run
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/kolchak
git diff --check
```

Expected: `0 issues` from golangci-lint and no test, race, vet, or build failures.

- [ ] **Step 6: Commit cancellation evidence**

```bash
git add internal/agent/httpagent
git commit -m "test(httpagent): verify context cancellation"
```

- [ ] **Step 7: Verify story-only scope**

Run:

```bash
git diff --stat main...HEAD
git log --oneline main..HEAD
```

Expected: only the approved design, this plan, shared JSON tags, and HTTP adapter implementation/tests appear. No command-adapter or later-story work is included.
