# Command Agent Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a configured, bounded, cancellation-aware local subprocess adapter implementing `agent.Agent`.

**Architecture:** Extend the config schema with a command argument list and add a focused `internal/agent/commandagent` package. The adapter sends the shared request as one JSON document on stdin, captures stdout and stderr concurrently with limits, strictly decodes one result from stdout after a successful exit, and maps failures into the existing agent taxonomy.

**Tech Stack:** Go 1.26+, `os/exec`, `context`, `encoding/json`, `io`, and Go test helper processes.

**Spec:** `docs/superpowers/specs/2026-09-03-command-agent-adapter-design.md`

## Global Constraints

- Rebase this branch onto `main` only after KOLCHAK-7 has merged; KOLCHAK-7 supplies the shared `agent.Request`, `agent.Result`, and `agent.Event` JSON tags.
- Accept command configuration only as an explicit `[]string`; never invoke a shell or parse a command string.
- The subprocess protocol is one JSON request on stdin and one JSON result on stdout; stderr is diagnostics only.
- The caller's `context.Context` is the sole timeout and cancellation policy.
- Cap stdout and stderr independently at 1 MiB.
- Reject unknown JSON fields and additional stdout JSON documents.
- Nonzero exits must not disclose stdout; include only bounded stderr diagnostics.
- Context cancellation and deadline errors must match `ctx.Err()` via `errors.Is` and must not be classified as `agent.AdapterError`.
- Do not add retries, streaming, environment configuration, working-directory configuration, process-group management, or CLI execution wiring.
- Tests use the Go test binary as a helper process and never invoke a shell or external service.
- The branch and PR contain only KOLCHAK-8 work.

---

### Task 1: Add command configuration and validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/validate.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/validate_test.go`

**Interfaces:**
- Produces `config.Agent.Command []string` with YAML key `command`.
- Produces support for `agent.type: command` requiring a non-empty command slice whose entries are non-empty after trimming whitespace.

- [ ] **Step 1: Add failing command-validation tests**

Add these cases to `TestValidate` in `internal/config/validate_test.go`:

```go
{
    name: "command agent without command",
    config: Config{Agent: Agent{Type: "command"}},
    want:  "agent.command: is required for the command adapter",
},
{
    name: "command agent with blank executable",
    config: Config{Agent: Agent{Type: "command", Command: []string{" "}}},
    want:  "agent.command[0]: must not be empty",
},
{
    name: "command agent with blank argument",
    config: Config{Agent: Agent{Type: "command", Command: []string{"agent", " "}}},
    want:  "agent.command[1]: must not be empty",
},
```

Add a table-driven `TestValidateCommandAgent` covering a valid command such as `[]string{"agent", "--stdio"}` and invalid empty or blank entries.

- [ ] **Step 2: Run the focused validation tests and verify RED**

Run: `go test ./internal/config -run 'TestValidate(CommandAgent)?$' -count=1`

Expected: FAIL because `Agent` has no `Command` field and `command` is unsupported.

- [ ] **Step 3: Add the schema field and command validator**

In `internal/config/config.go`, extend `Agent`:

```go
type Agent struct {
    Type    string   `yaml:"type"`
    URL     string   `yaml:"url"`
    Command []string `yaml:"command"`
}
```

In `Config.Validate`, add a `case "command":` that calls:

```go
func validateCommandAgent(validation *ValidationError, agent Agent) {
    if len(agent.Command) == 0 {
        validation.add("agent.command", "is required for the command adapter")
        return
    }
    for i, argument := range agent.Command {
        if strings.TrimSpace(argument) == "" {
            validation.add(fmt.Sprintf("agent.command[%d]", i), "must not be empty")
        }
    }
}
```

Update the unsupported-adapter diagnostic to name both supported adapters.

- [ ] **Step 4: Prove YAML loading preserves command arguments**

Add a `Load` fixture with:

```yaml
agent:
  type: command
  command: [agent, --stdio]
```

Assert `cfg.Agent.Command` equals `[]string{"agent", "--stdio"}`.

- [ ] **Step 5: Run the config suite and verify GREEN**

Run: `go test ./internal/config -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config
git commit -m "feat(config): validate command agents"
```

### Task 2: Implement successful command exchange and strict bounded output

**Files:**
- Create: `internal/agent/commandagent/commandagent.go`
- Create: `internal/agent/commandagent/commandagent_test.go`

**Interfaces:**
- Consumes `agent.Agent`, `agent.Request`, `agent.Result`, and KOLCHAK-7's JSON tags.
- Produces `New(argv []string) (*Adapter, error)` and `(*Adapter).Run(context.Context, agent.Request) (agent.Result, error)`.

- [ ] **Step 1: Write the helper-process fixture and all failing success-path tests**

Create a `TestHelperProcess` that reads its mode from the first argument after
`--`. The test command targets only this test with `-test.run=TestHelperProcess`,
so it needs no environment-variable guard. Add a helper:

```go
func helperCommand(mode string) []string {
    return []string{os.Args[0], "-test.run=TestHelperProcess", "--", mode}
}
```

Add `TestNewRejectsInvalidCommand` for `nil`, `[]string{}`, and `[]string{" "}`.

In helper mode `success`, decode `agent.Request` from stdin and require `Task == "Fix the failing test"`. Write this literal result to stdout:

```json
{"events":[{"type":"tool_call","data":{"name":"verify"}}],"output":"fixed"}
```

`TestRunExchangesJSONWithCommand` must call `Run`, assert the literal event and output, and prove the command receives `--stdio` as an uninterpreted argv entry rather than shell syntax.

Add helper modes and a table-driven `TestRunClassifiesInvalidOutput` for these
literal stdout bodies:

```go
`{"events":`,
`{"events":[],"output":"","extra":true}`,
`{"events":[],"output":""}{}`,
```

Add a mode that writes `maxOutputBytes+1` bytes to stdout. Assert
`agent.FailureInvalidResponse` for every invalid-output case.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/agent/commandagent -run 'Test(NewRejectsInvalidCommand|Run(ExchangesJSONWithCommand|ClassifiesInvalidOutput))' -count=1`

Expected: FAIL because the package and adapter do not exist.

- [ ] **Step 3: Implement constructor, process startup, bounded capture, and strict decoding**

Create `commandagent.go` with:

```go
const maxOutputBytes = 1 << 20

type Adapter struct {
    argv []string
}

var _ agent.Agent = (*Adapter)(nil)

func New(argv []string) (*Adapter, error) {
    if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
        return nil, errors.New("command agent requires a non-empty command")
    }
    copied := append([]string(nil), argv...)
    return &Adapter{argv: copied}, nil
}
```

Use `exec.CommandContext(ctx, a.argv[0], a.argv[1:]...)`, obtain stdout,
stderr, and stdin pipes before `Start`, and run bounded reads of stdout and
stderr concurrently. Marshal and write `agent.Request` to stdin, close stdin,
wait for the process and both reads, then decode stdout only after a successful
exit. Implement a private bounded reader that reads at most `maxOutputBytes+1`
and reports whether the limit was exceeded.

Decode `agent.Result` from a `bytes.Reader` with `DisallowUnknownFields`, then
decode a second document and require `io.EOF`. Treat an oversized stdout,
malformed JSON, unknown field, or extra document as
`agent.AdapterError{Kind: agent.FailureInvalidResponse}`.

- [ ] **Step 4: Run package tests and verify GREEN**

Run: `go test ./internal/agent/commandagent -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/commandagent
git commit -m "feat(commandagent): exchange bounded JSON"
```

### Task 3: Classify process failure and preserve cancellation

**Files:**
- Modify: `internal/agent/commandagent/commandagent.go`
- Modify: `internal/agent/commandagent/commandagent_test.go`

**Interfaces:**
- Consumes `agent.AdapterError`, `agent.FailureUnavailable`, `agent.FailureRejected`, and `agent.FailureKindOf`.
- Produces complete command-adapter error semantics.

- [ ] **Step 1: Write failing process-start and nonzero-exit tests**

Add `TestRunClassifiesStartFailure` using `New([]string{"definitely-not-a-kolchak-command"})` and assert `FailureUnavailable`.

Add helper mode `exit` that writes `private diagnostic` to stderr and exits
with status 7. `TestRunRejectsNonzeroExit` must assert `FailureRejected`, the
error contains the exit status and bounded stderr, and it does not contain any
stdout result data.

- [ ] **Step 2: Run focused failure tests and verify RED**

Run: `go test ./internal/agent/commandagent -run 'TestRun(ClassifiesStartFailure|RejectsNonzeroExit)' -count=1`

Expected: FAIL because start and exit errors are not classified.

- [ ] **Step 3: Map start and exit failures**

Wrap a `Start` failure in:

```go
&agent.AdapterError{Kind: agent.FailureUnavailable, Err: err}
```

For a non-context `Wait` failure, return:

```go
&agent.AdapterError{
    Kind: agent.FailureRejected,
    Err:  fmt.Errorf("command agent exited: %w: %s", err, boundedStderr),
}
```

Do not include stdout in the error. If stderr is empty, omit the trailing
diagnostic separator.

- [ ] **Step 4: Write failing cancellation and deadline tests**

Add helper mode `block` that closes a test-visible started signal and waits
until its context ends. In `TestRunPreservesCancellation`, cancel a context
after observing process start, then assert `errors.Is(err, context.Canceled)`
and that `FailureKindOf(err)` is false.

In `TestRunPreservesDeadline`, use `context.WithTimeout`, wait for helper
start with a bounded `select`, and assert `errors.Is(err,
context.DeadlineExceeded)` with no classification.

- [ ] **Step 5: Run cancellation tests and verify RED**

Run: `go test ./internal/agent/commandagent -run 'TestRunPreserves(Cancellation|Deadline)' -count=1`

Expected: FAIL because the process exit produced by `CommandContext` is being
classified as rejected.

- [ ] **Step 6: Give context errors precedence**

After `Wait` and pipe reads complete, return `ctx.Err()` before constructing
an `AdapterError` whenever it is non-nil. This check must cover cancellation
that races with process exit, stdin write, or pipe reads.

- [ ] **Step 7: Run the full local quality gate**

Run each command and require exit zero:

```bash
gofmt -w internal/config/config.go internal/config/validate.go internal/config/config_test.go internal/config/validate_test.go internal/agent/commandagent/commandagent.go internal/agent/commandagent/commandagent_test.go
golangci-lint fmt --diff
golangci-lint run
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/kolchak
git diff --check
```

- [ ] **Step 8: Verify story-only scope and commit**

Run:

```bash
git diff --stat main...HEAD
git log --oneline main..HEAD
```

Confirm only the approved design, this plan, config changes, and command
adapter changes exist. Then commit:

```bash
git add internal/agent/commandagent internal/config
git commit -m "feat(commandagent): classify process failures"
```
