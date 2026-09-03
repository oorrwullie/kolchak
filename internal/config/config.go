package config

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Version    int      `yaml:"version"`
	Agent      Agent    `yaml:"agent"`
	Tests      Tests    `yaml:"tests"`
	Faults     []string `yaml:"faults"`
	Properties []string `yaml:"properties"`
	Runs       Runs     `yaml:"runs"`
}

type Agent struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

type Tests struct {
	Inputs []string `yaml:"inputs"`
}

type Runs struct {
	Confirm     int `yaml:"confirm"`
	Concurrency int `yaml:"concurrency"`
}

func Default() Config {
	return Config{
		Version: 1,
		Agent: Agent{
			Type: "http",
			URL:  "http://localhost:8080/agent",
		},
		Tests:      Tests{Inputs: []string{"Fix the failing test"}},
		Faults:     []string{"timeout", "error", "malformed"},
		Properties: []string{"verification_required"},
		Runs: Runs{
			Confirm:     10,
			Concurrency: 4,
		},
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := Default()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Config{}, fmt.Errorf("decode config %q: %w", path, err)
		}
		return Config{}, fmt.Errorf("decode config %q: multiple YAML documents are not supported", path)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}

	return cfg, nil
}
