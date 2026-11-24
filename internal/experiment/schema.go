// Package experiment defines the AgentChaos experiment file: its schema,
// defaults, path resolution and validation.
package experiment

import (
	"strings"
	"time"
)

// File is a parsed experiment file.
type File struct {
	Path          string            `yaml:"-"`
	Agent         Agent             `yaml:"agent"`
	Servers       map[string]Server `yaml:"servers"`
	Runs          int               `yaml:"runs"`
	RunTimeout    time.Duration     `yaml:"run_timeout"`
	PassThreshold float64           `yaml:"pass_threshold"`
	Experiments   []Experiment      `yaml:"experiments"`
}

// Agent is the agent under test: exactly one of Preset or Command.
type Agent struct {
	Preset  string            `yaml:"preset"`
	Command []string          `yaml:"command"`
	Env     map[string]string `yaml:"env"`
}

// Server is a real MCP server the proxy fronts.
type Server struct {
	Command []string          `yaml:"command"`
	Env     map[string]string `yaml:"env"`
}

// Experiment is one fault plus the behaviour expected of the agent.
type Experiment struct {
	Name   string `yaml:"name"`
	Task   string `yaml:"task"`
	Fault  Fault  `yaml:"fault"`
	Expect Expect `yaml:"expect"`
}

// Fault describes what to inject and when.
type Fault struct {
	Tool        string        `yaml:"tool"` // <server>.<tool>
	Type        string        `yaml:"type"`
	OnCall      int           `yaml:"on_call"`
	EveryCall   bool          `yaml:"every_call"`
	Probability float64       `yaml:"probability"`
	Seed        int64         `yaml:"seed"`
	Delay       time.Duration `yaml:"delay"`
	ErrorMode   string        `yaml:"error_mode"` // tool (default) | jsonrpc
	Message     string        `yaml:"message"`
	Fraction    float64       `yaml:"fraction"`
	Bytes       int           `yaml:"bytes"`
	Corrupt     string        `yaml:"corrupt"` // invalid-json (default) | drop-fields | wrong-types
}

// Server returns the server part of Tool (before the first dot).
func (f Fault) Server() string {
	s, _, _ := strings.Cut(f.Tool, ".")
	return s
}

// ToolName returns the tool part of Tool (after the first dot).
func (f Fault) ToolName() string {
	_, t, _ := strings.Cut(f.Tool, ".")
	return t
}

// Expect holds the deterministic checks applied to a run.
type Expect struct {
	MaxCalls         *MaxCalls `yaml:"max_calls"`
	Backoff          bool      `yaml:"backoff"`
	MustCall         []string  `yaml:"must_call"`
	MustNotCall      []string  `yaml:"must_not_call"`
	StopsWithin      *int      `yaml:"stops_within"`
	OutputMatches    string    `yaml:"output_matches"`
	OutputNotMatches string    `yaml:"output_not_matches"`
	ExitCode         *int      `yaml:"exit_code"`
}

// MaxCalls bounds the number of calls to a tool (<server>.<tool>).
type MaxCalls struct {
	Tool  string `yaml:"tool"`
	Count int    `yaml:"count"`
}
