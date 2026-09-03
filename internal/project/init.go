package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigName = "kolchak.yaml"

const defaultConfig = `version: 1

agent:
  type: http
  url: http://localhost:8080/agent

tests:
  inputs:
    - Fix the failing test

faults:
  - timeout
  - error
  - malformed

properties:
  - verification_required

runs:
  confirm: 10
  concurrency: 4
`

func Init(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	configPath := filepath.Join(dir, ConfigName)
	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists", configPath)
	}
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	if _, err := f.WriteString(defaultConfig); err != nil {
		_ = f.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}

	for _, name := range []string{"runs", "failures", "cases"} {
		if err := os.MkdirAll(filepath.Join(dir, ".kolchak", name), 0o755); err != nil {
			return fmt.Errorf("create %s directory: %w", name, err)
		}
	}
	return nil
}
