package casefile

type Case struct {
	Version      int
	Name         string
	Input        Input
	Faults       []Fault
	Property     string
	Reproduction Reproduction
	Metadata     Metadata
}

type Input struct{ Task string }

type Fault struct {
	Tool string
	Type string
}

type Reproduction struct {
	Attempts int
	Failures int
}

type Metadata struct {
	DiscoveredBy string
	Seed         int64
}
