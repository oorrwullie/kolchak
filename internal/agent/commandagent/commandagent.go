// Package commandagent adapts the agent contract to a local JSON-speaking command.
package commandagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/oorrwullie/kolchak/internal/agent"
)

const maxOutputBytes = 1 << 20

// Adapter sends agent requests to a configured local command.
type Adapter struct {
	argv []string
}

var _ agent.Agent = (*Adapter)(nil)

// New creates an Adapter for an explicit command argument list.
func New(argv []string) (*Adapter, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, errors.New("command agent requires a non-empty command")
	}

	copied := append([]string(nil), argv...)
	return &Adapter{argv: copied}, nil
}

// Run sends req to the command and returns its JSON result.
func (a *Adapter) Run(ctx context.Context, req agent.Request) (agent.Result, error) {
	if err := ctx.Err(); err != nil {
		return agent.Result{}, err
	}

	request, err := json.Marshal(req)
	if err != nil {
		return agent.Result{}, fmt.Errorf("marshal agent request: %w", err)
	}

	command := exec.CommandContext(ctx, a.argv[0], a.argv[1:]...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return agent.Result{}, fmt.Errorf("create command stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return agent.Result{}, fmt.Errorf("create command stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdin.Close()
		return agent.Result{}, fmt.Errorf("create command stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stderr.Close()
		_ = stdout.Close()
		_ = stdin.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return agent.Result{}, ctxErr
		}
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureUnavailable,
			Err:  err,
		}
	}

	type capturedOutput struct {
		body     []byte
		exceeded bool
		err      error
	}
	stdoutResult := make(chan capturedOutput, 1)
	stderrResult := make(chan capturedOutput, 1)
	go func() {
		body, exceeded, err := readBounded(stdout)
		stdoutResult <- capturedOutput{body: body, exceeded: exceeded, err: err}
	}()
	go func() {
		body, exceeded, err := readBounded(stderr)
		stderrResult <- capturedOutput{body: body, exceeded: exceeded, err: err}
	}()

	if _, err := stdin.Write(request); err != nil {
		_ = stdin.Close()
		_ = command.Wait()
		<-stdoutResult
		<-stderrResult
		if ctxErr := ctx.Err(); ctxErr != nil {
			return agent.Result{}, ctxErr
		}
		return agent.Result{}, &agent.AdapterError{
			Kind: agent.FailureUnavailable,
			Err:  fmt.Errorf("write command request: %w", err),
		}
	}
	closeErr := stdin.Close()

	waitErr := command.Wait()
	stdoutCapture := <-stdoutResult
	stderrCapture := <-stderrResult
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.Result{}, ctxErr
	}
	if waitErr != nil {
		return agent.Result{}, rejectedError(waitErr, stderrCapture.body)
	}
	if closeErr != nil {
		return agent.Result{}, fmt.Errorf("close command stdin: %w", closeErr)
	}
	if stdoutCapture.err != nil {
		return agent.Result{}, invalidResponseError(fmt.Errorf("read command response: %w", stdoutCapture.err))
	}
	if stdoutCapture.exceeded {
		return agent.Result{}, invalidResponseError(errors.New("command response exceeds maximum size"))
	}

	return decodeResult(stdoutCapture.body)
}

func readBounded(reader io.Reader) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxOutputBytes+1))
	return body, len(body) > maxOutputBytes, err
}

func decodeResult(body []byte) (agent.Result, error) {
	var result agent.Result
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return agent.Result{}, invalidResponseError(fmt.Errorf("decode command response: %w", err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("command response contains multiple JSON documents")
		}
		return agent.Result{}, invalidResponseError(fmt.Errorf("decode command response: %w", err))
	}
	return result, nil
}

func invalidResponseError(err error) error {
	return &agent.AdapterError{Kind: agent.FailureInvalidResponse, Err: err}
}

func rejectedError(err error, stderr []byte) error {
	if len(stderr) > maxOutputBytes {
		stderr = stderr[:maxOutputBytes]
	}
	diagnostic := strings.TrimSpace(string(stderr))
	if diagnostic == "" {
		return &agent.AdapterError{
			Kind: agent.FailureRejected,
			Err:  fmt.Errorf("command agent exited: %w", err),
		}
	}
	return &agent.AdapterError{
		Kind: agent.FailureRejected,
		Err:  fmt.Errorf("command agent exited: %w: %s", err, diagnostic),
	}
}
