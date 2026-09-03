# Kolchak

**Reliability testing for AI agents.**

> Your agent says it worked. Kolchak checks.

Kolchak is a local-first developer tool for finding behavioral failures
in AI agents, reducing those failures to reproducible cases, and keeping
them from coming back.

Most agent testing asks whether an agent *can* succeed.

Kolchak asks a different question:

**What happens when things go wrong?**

Tools time out. APIs fail. Responses arrive malformed or incomplete.
Verification becomes unavailable. Context changes. An agent may
encounter all of those conditions and still confidently report that it
completed the task.

Kolchak deliberately creates those conditions and checks what the agent
actually does.

When it discovers a failure, it confirms that the failure is
reproducible, minimizes the conditions required to trigger it, and saves
the result as a human-readable test case that can be replayed locally or
run in CI.

The goal is simple:

> **Don't test whether your agent can succeed. Test whether it knows
> when it hasn't.**

------------------------------------------------------------------------

## Status

**Kolchak is currently under development.**

The initial implementation is being built around the workflow and file
formats described below. Commands and configuration shown in this README
represent the intended v0.1 interface and may change while the first
release is being developed.

------------------------------------------------------------------------

## The problem

AI agents fail differently from conventional software.

A normal program usually has a fairly clear relationship between input,
execution, and output. An agent operates inside a changing environment:

-   model responses vary;
-   tools can fail;
-   APIs can become unavailable;
-   responses can be delayed or malformed;
-   intermediate assumptions can be wrong;
-   verification can fail independently of the task;
-   multiple individually reasonable decisions can combine into a bad
    outcome.

Traditional unit tests are necessary, but they don't exercise much of
that surface.

Observability helps explain a failure **after you know one happened**.

Evaluations help measure how often an agent succeeds on a known task.

Kolchak is intended to complement both by actively searching for
conditions under which the agent behaves incorrectly.

It doesn't just ask:

> Did the agent complete the task?

It can ask questions like:

> Did the agent claim success after its verification tool failed?

> Did it continue after an operation that required approval?

> Did it treat a malformed tool response as valid evidence?

> Did it distinguish between "the task succeeded" and "I was unable to
> verify the task"?

Those are behavioral reliability properties.

------------------------------------------------------------------------

## The workflow

The intended basic workflow is deliberately small:

``` console
$ kolchak init
$ kolchak test
```

A test run might eventually look something like this:

``` text
$ kolchak test

Running experiments...

✓ baseline
✓ tool unavailable
✓ malformed response
✓ rate limited
✗ verification timeout

Agent reported task completion after verification failed.

Confirming failure...
19/20 runs reproduced the violation.

Minimizing...
6 environmental changes → 1

Minimal trigger:
  verify → timeout

Saved:
  .kolchak/failures/verification-timeout.case

Reproduce:
  kolchak replay .kolchak/failures/verification-timeout.case
```

The important part isn't the injected timeout.

It's what happens next.

Kolchak turns an observed behavioral failure into a **small,
reproducible artifact**.

After fixing the agent:

``` console
$ kolchak replay .kolchak/failures/verification-timeout.case

PASS
```

The case can then become a permanent regression test:

``` console
$ kolchak accept .kolchak/failures/verification-timeout.case
$ kolchak ci
```

The complete loop is:

``` text
explore
   ↓
discover
   ↓
confirm
   ↓
minimize
   ↓
reproduce
   ↓
fix
   ↓
regress
```

------------------------------------------------------------------------

## Failure minimization

Failure injection by itself is useful, but it can produce noisy results.

Suppose Kolchak changes six things in an agent's environment and
discovers a behavioral violation.

Which change mattered?

Was it one condition?

Two interacting conditions?

Were four of them irrelevant?

Kolchak's defining feature is intended to be **failure minimization**.

Once a candidate failure is found, Kolchak repeatedly removes or
simplifies environmental changes while checking whether the failure
still reproduces.

The goal is to turn this:

``` text
tool latency increased
verification timeout
API returned partial data
secondary tool unavailable
context truncated
retry delayed
```

into this:

``` text
verify → timeout
```

That minimal trigger is dramatically easier to understand, fix, discuss
in a pull request, and preserve as a regression test.

------------------------------------------------------------------------

## `.case` files

The fundamental artifact in Kolchak is not a trace.

It's a **case**.

Cases are intended to be:

-   human-readable;
-   deterministic where Kolchak controls the environment;
-   portable;
-   editable;
-   diffable;
-   easy to review in Git;
-   runnable without a hosted service.

A case may look like:

``` yaml
version: 1
name: verification-timeout

input:
  task: "Fix the failing test."

faults:
  - tool: verify
    type: timeout
    after: 2s

property:
  verification_required

reproduction:
  attempts: 20
  failures: 19

metadata:
  discovered_by: kolchak
  seed: 918234
```

Cases discovered during exploration live under:

``` text
.kolchak/failures/
```

Accepted regression cases live under:

``` text
.kolchak/cases/
```

That makes the failure itself something you can commit alongside the
fix.

------------------------------------------------------------------------

## Behavioral properties

Kolchak isn't intended to decide whether an agent's prose is "good."

It tests behaviors that matter to the application.

For example:

``` yaml
properties:
  - name: verification_required
    when:
      action: complete
    require:
      tool_success: verify
```

This expresses a simple invariant:

> The agent must not report completion unless verification succeeded.

Other properties might eventually express requirements such as:

-   deployment requires approval;
-   destructive actions require confirmation;
-   failed authentication must stop execution;
-   unavailable evidence must not be represented as verified evidence;
-   a failed write must not be reported as successful;
-   certain tools must execute before completion.

The goal is to make important behavioral expectations explicit and
executable.

------------------------------------------------------------------------

## Fault injection

Kolchak will start with a deliberately small set of environmental
failures.

Initial fault classes are expected to include:

### Timing

-   delay
-   timeout

### Availability

-   error
-   unavailable
-   rate limit

### Response integrity

-   empty response
-   malformed response
-   partial response

Later versions may explore conditions such as stale or contradictory
data, reordered or duplicated responses, context pressure, cancellation,
semantic corruption, and interacting failures.

The purpose isn't to create the largest possible fault catalog.

It's to find **small failures that expose unsafe assumptions in agent
behavior**.

------------------------------------------------------------------------

## Framework agnostic

Kolchak is not an agent framework.

It should not own your agent's lifecycle, model selection, memory
architecture, workflow graph, or tool implementation.

**You own the agent. Kolchak tests it.**

The initial adapters are planned around simple boundaries such as:

-   HTTP;
-   commands/subprocesses.

Additional adapters can come later for common agent protocols and
frameworks.

The architecture intentionally keeps Kolchak outside the application
under test.

------------------------------------------------------------------------

## Local first

Agent traces can contain source code, prompts, customer data, internal
tool responses, credentials, or other information that developers may
not want sent to another service.

Kolchak is therefore designed as a **local-first tool**.

The initial goal is a single Go binary with:

-   no required cloud account;
-   no required database;
-   no required Python runtime;
-   no required Docker environment.

Runs, discovered failures, and accepted cases remain ordinary local
files.

Hosted or distributed capabilities may make sense someday. They are not
required for the core idea to work.

------------------------------------------------------------------------

## Reproducibility, not fake determinism

LLM-backed systems are inherently variable.

Kolchak won't pretend otherwise.

Instead, it aims to make everything **under its control** reproducible:

-   injected faults;
-   fault parameters;
-   seeds;
-   test inputs;
-   behavioral properties;
-   environmental changes.

Candidate failures can be rerun repeatedly to estimate whether a
violation is actually reproducible rather than a one-off model outcome.

For example:

``` text
Failure reproduced: 19/20 runs
Estimated reproduction rate: 95%
```

The first implementation doesn't need sophisticated statistics. It needs
enough evidence to distinguish a persistent failure from noise.

------------------------------------------------------------------------

## CI

Accepted cases should behave like ordinary regression tests.

``` console
$ kolchak ci
```

A CI run should replay the cases under `.kolchak/cases/` and return a
non-zero exit status when a behavioral regression is detected.

The intention is that a production failure discovered once can become a
test that runs forever.

``` text
production failure
       ↓
reproduced case
       ↓
fix
       ↓
regression test
       ↓
CI
```

------------------------------------------------------------------------

## What Kolchak is not

Kolchak is intentionally **not**:

-   another agent runtime;
-   another LLM framework;
-   an agent builder;
-   a prompt-management platform;
-   a generic evaluation platform;
-   a tracing/observability backend;
-   a hosted SaaS requirement;
-   a benchmark suite;
-   a general-purpose security scanner.

There are already excellent tools in many of those categories.

Kolchak is focused on a narrower problem:

**Find ways an agent can behave incorrectly under failure, reduce those
failures to reproducible cases, and make sure they stay fixed.**

------------------------------------------------------------------------

## Planned v0.1

The first useful version is intentionally constrained.

The target is:

-   [ ] installable Go binary;
-   [ ] `kolchak init`;
-   [ ] small `kolchak.yaml` configuration;
-   [ ] HTTP agent adapter;
-   [ ] command/subprocess adapter;
-   [ ] observable tool-call boundary;
-   [ ] timeout fault;
-   [ ] tool-error fault;
-   [ ] malformed-response fault;
-   [ ] at least one behavioral property;
-   [ ] bounded concurrent experiments;
-   [ ] repeated failure confirmation;
-   [ ] automatic failure minimization;
-   [ ] human-readable `.case` files;
-   [ ] `kolchak replay`;
-   [ ] `kolchak accept`;
-   [ ] `kolchak ci`;
-   [ ] useful exit codes;
-   [ ] one end-to-end demonstration that reliably finds a real
    behavioral failure.

If v0.1 requires a dashboard, distributed workers, a database, or a
dozen framework integrations before it becomes useful, the scope is
wrong.

------------------------------------------------------------------------

## Example target

A simple coding agent provides a good demonstration.

Under normal conditions:

``` text
User: Fix the failing test.

Agent:
  → modifies file
  → run_tests
  ← PASS
  → reports: "Fixed. All tests pass."
```

Kolchak changes one part of the environment:

``` text
run_tests → timeout
```

A poorly designed agent may still respond:

``` text
Fixed. All tests pass.
```

The code change might even be correct.

The behavioral failure is that the agent **claimed verification it did
not have**.

Kolchak should:

1.  detect the property violation;
2.  reproduce it across repeated runs;
3.  minimize the environment to the timeout that actually matters;
4.  save the failure as a `.case`;
5.  replay that exact case while the agent is being fixed;
6.  preserve the case as a permanent regression test.

A corrected agent might instead respond:

``` text
I made the change, but the test runner timed out, so I couldn't verify that
the tests pass.
```

Same environmental failure.

Correct behavior.

------------------------------------------------------------------------

## Design principles

A few principles guide the project:

**The developer owns the agent.**\
Kolchak should integrate with an agent rather than become the framework
the agent must run inside.

**Failures should become artifacts.**\
A discovered failure is much more valuable when it can be committed,
shared, replayed, and reviewed.

**Minimize before explaining.**\
The smallest reproducible trigger is often more useful than a
sophisticated explanation of a noisy experiment.

**Evidence beats confidence.**\
An agent sounding certain is not evidence that an operation succeeded.

**Local should be enough.**\
The core workflow should not depend on external infrastructure.

**Simple commands matter.**\
The common path should remain:

``` console
kolchak test
kolchak replay ...
kolchak ci
```

------------------------------------------------------------------------

## Why "Kolchak"?

The name is a nod to **Carl Kolchak**, the investigative reporter from
the 1970s cult series *Kolchak: The Night Stalker*.

Kolchak had a habit of investigating cases where the accepted
explanation didn't match the evidence. He kept digging until he found
what actually happened.

That's a pretty good description of the job this project is meant to do.

An AI agent says it completed the task successfully. Kolchak doesn't
take the claim at face value. It changes the environment, follows the
evidence, reproduces what went wrong, and reduces the failure to a case
you can keep.

There's some extra nerd pedigree, too: *Kolchak: The Night Stalker* was
an important inspiration for Chris Carter when he created *The X-Files*.

And, appropriately enough, one episode of *Kolchak* involved an
artificial-intelligence-equipped robot behaving rather badly.

Knowing any of that is entirely optional.

**Kolchak is the investigator. Your agent is the witness. The evidence
decides.**

------------------------------------------------------------------------

## Project philosophy

AI agents are becoming capable enough that "it usually works" is no
longer a sufficient reliability strategy.

The interesting failures increasingly happen between components:

-   the model and its tools;
-   an action and its verification;
-   a tool result and the agent's interpretation of it;
-   an expected environment and the environment that actually exists.

Those boundaries deserve adversarial testing.

Kolchak is an experiment in making that kind of testing feel less like
reliability research and more like ordinary software development:

``` console
kolchak test
```

Find the failure.

Make it small.

Fix it.

Keep the case.

------------------------------------------------------------------------

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
