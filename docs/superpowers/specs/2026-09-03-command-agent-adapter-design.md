# Command Agent Adapter Design

## Purpose

`KOLCHAK-8` adds Kolchak's v0.1 local-command adapter. It runs a configured
agent program for one transport-neutral `agent.Request`, turns its JSON output
into `agent.Result`, and keeps process details out of the experiment engine.

## Decision

Add `internal/agent/commandagent` with a concrete `agent.Agent`:

```go
type Adapter struct { /* private fields */ }

func New(argv []string) (*Adapter, error)
func (a *Adapter) Run(ctx context.Context, request agent.Request) (agent.Result, error)
```

The configured command is an explicit argument list. `argv[0]` is the program
and remaining entries are its arguments. The adapter does not invoke a shell,
parse a command string, interpolate variables, or add authentication,
streaming, retries, or working-directory configuration.

The adapter sends exactly one JSON request on standard input:

```json
{"task":"Fix the failing test"}
```

It accepts exactly one JSON result on standard output:

```json
{
  "events": [{"type":"tool_call","data":{"name":"verify"}}],
  "output":"The test is fixed."
}
```

Standard error is reserved for program diagnostics and is never decoded as a
result. The adapter closes stdin after writing the request so commands can
recognize the end of their input.

## Configuration

Extend the existing `agent` configuration with an explicit command field:

```yaml
agent:
  type: command
  command: ["my-agent", "--kolchak-stdio"]
```

`agent.type: http` retains its current URL validation and ignores `command`.
For `agent.type: command`, `agent.command` must contain at least one
non-empty string. The default configuration remains the existing HTTP example;
this story does not change command wiring into a future test-run CLI command.

## Execution and Boundaries

`Run` creates the process with `exec.CommandContext`, giving the caller's
context sole ownership of deadlines and cancellation. On cancellation or a
deadline, it returns an error matching `ctx.Err()` with `errors.Is` and never
wraps that condition in `agent.AdapterError`.

The adapter captures stdout and stderr concurrently so a verbose child cannot
block on one pipe while the other is read. Each stream is limited to 1 MiB.
An oversized stdout result is `agent.FailureInvalidResponse`; stderr remains
bounded diagnostic context only. The adapter requires a successful exit before
accepting stdout as a result.

Successful stdout is decoded with the same strict protocol boundary as the
HTTP adapter: unknown fields and additional JSON documents are rejected.

## Error Mapping

- Invalid or empty command configuration: `New` returns an unclassified local
  validation error.
- Process-start or request-write failure: `agent.FailureUnavailable`.
- Nonzero process exit: `agent.FailureRejected`; the diagnostic contains a
  bounded stderr excerpt but not stdout.
- Oversized, malformed, unknown-field, or multi-document stdout: 
  `agent.FailureInvalidResponse`.
- Context cancellation or deadline: the direct context error, unclassified.

The adapter does not retry an execution. Running an agent twice can duplicate
side effects, so retry policy belongs to a later engine story.

## Testing

Tests use the Go test binary as a helper subprocess and an explicit argument
list. They cover:

- command validation and configuration validation;
- a successful request/result exchange, including stdin JSON and stdout JSON;
- nonzero exit with bounded stderr diagnostics;
- malformed, unknown-field, multi-document, and oversized stdout;
- context cancellation and deadline expiration;
- no shell interpretation of command arguments.

The helpers perform no external network calls and no shell invocation. Tests
must ensure a cancellation or deadline produces no adapter classification.

## Scope

This story delivers the adapter and configuration schema only. It excludes a
new CLI test command, process trees or platform-specific child-group cleanup,
environment-variable configuration, working-directory selection, retrying,
streaming, and a shared contract-test suite for both adapters.
