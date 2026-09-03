package config

type Config struct {
	Version    int
	Agent      Agent
	Tests      Tests
	Faults     []string
	Properties []string
	Runs       Runs
}

type Agent struct {
	Type string
	URL  string
}

type Tests struct{ Inputs []string }

type Runs struct {
	Confirm     int
	Concurrency int
}
