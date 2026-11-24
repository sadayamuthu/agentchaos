package experiment

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

var faultTypes = map[string]bool{"timeout": true, "slow": true, "error": true, "partial": true, "malformed": true}

// Load reads, defaults, path-resolves and validates an experiment file.
// It does not check that the commands exist.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

// Parse is Load with the file's contents, data, already read from path.
// Relative command paths resolve against path's directory.
func Parse(path string, data []byte) (*File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	f := &File{Runs: 1, RunTimeout: 5 * time.Minute, PassThreshold: 1.0}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.Path = abs
	applyDefaults(f)
	dir := filepath.Dir(abs)
	resolveCommand(f.Agent.Command, dir)
	for _, s := range f.Servers {
		resolveCommand(s.Command, dir)
	}
	if err := f.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func applyDefaults(f *File) {
	for i := range f.Experiments {
		ft := &f.Experiments[i].Fault
		if ft.ErrorMode == "" {
			ft.ErrorMode = "tool"
		}
		if ft.Message == "" {
			ft.Message = "internal error"
		}
		if ft.Fraction == 0 && ft.Bytes == 0 {
			ft.Fraction = 0.5
		}
		if ft.Corrupt == "" {
			ft.Corrupt = "invalid-json"
		}
	}
}

// resolveCommand makes cmd[0] absolute (relative to dir) if it contains a
// slash. Bare names are left for PATH lookup at run time.
func resolveCommand(cmd []string, dir string) {
	if len(cmd) == 0 || !strings.Contains(cmd[0], "/") || filepath.IsAbs(cmd[0]) {
		return
	}
	cmd[0] = filepath.Join(dir, cmd[0])
}

// Validate returns every problem found, joined.
func (f *File) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	hasPreset, hasCmd := f.Agent.Preset != "", len(f.Agent.Command) > 0
	switch {
	case hasPreset == hasCmd:
		add("agent: exactly one of preset or command is required")
	case hasPreset && f.Agent.Preset != "claude-code":
		add("agent.preset: unknown preset %q (only \"claude-code\" is supported)", f.Agent.Preset)
	}
	if f.Runs < 1 {
		add("runs: must be >= 1, got %d", f.Runs)
	}
	if f.PassThreshold <= 0 || f.PassThreshold > 1 {
		add("pass_threshold: must be in (0,1], got %v", f.PassThreshold)
	}
	if f.RunTimeout <= 0 {
		add("run_timeout: must be > 0")
	}
	for k, s := range f.Servers {
		switch {
		case k == "":
			add("servers: server key must not be empty")
		case strings.Contains(k, "."):
			add("servers[%q]: key must not contain \".\"", k)
		}
		if len(s.Command) == 0 {
			add("servers[%q].command: required", k)
		}
	}

	// toolRef checks a "<server>.<tool>" reference.
	toolRef := func(path, ref string) {
		srv, tool, ok := strings.Cut(ref, ".")
		if !ok || srv == "" || tool == "" {
			add("%s: %q must be <server>.<tool>", path, ref)
			return
		}
		if _, found := f.Servers[srv]; !found {
			add("%s: unknown server %q", path, srv)
		}
	}
	checkRegex := func(path, re string) {
		if re == "" {
			return
		}
		if _, err := regexp.Compile(re); err != nil {
			add("%s: invalid regex: %v", path, err)
		}
	}

	seen := map[string]bool{}
	for i, e := range f.Experiments {
		p := fmt.Sprintf("experiments[%d]", i)
		if e.Name == "" {
			add("%s.name: required", p)
		} else if seen[e.Name] {
			add("%s.name: duplicate name %q", p, e.Name)
		}
		seen[e.Name] = true
		if e.Task == "" {
			add("%s.task: required", p)
		}

		ft := e.Fault
		toolRef(p+".fault.tool", ft.Tool)
		if !faultTypes[ft.Type] {
			add("%s.fault.type: unknown type %q (want timeout, slow, error, partial or malformed)", p, ft.Type)
		}
		triggers := 0
		if ft.OnCall > 0 {
			triggers++
		}
		if ft.EveryCall {
			triggers++
		}
		if ft.Probability != 0 {
			triggers++
			if ft.Probability <= 0 || ft.Probability > 1 {
				add("%s.fault.probability: must be in (0,1], got %v", p, ft.Probability)
			}
		}
		if ft.OnCall < 0 {
			add("%s.fault.on_call: must be >= 1, got %d", p, ft.OnCall)
		}
		if triggers != 1 {
			add("%s.fault: exactly one trigger of on_call, every_call, probability is required, got %d", p, triggers)
		}
		if ft.Type == "slow" && ft.Delay <= 0 {
			add("%s.fault.delay: required (> 0) for type slow", p)
		}
		if ft.ErrorMode != "" && ft.ErrorMode != "tool" && ft.ErrorMode != "jsonrpc" {
			add("%s.fault.error_mode: unknown mode %q (want tool or jsonrpc)", p, ft.ErrorMode)
		}
		switch ft.Corrupt {
		case "", "invalid-json", "drop-fields", "wrong-types":
		default:
			add("%s.fault.corrupt: unknown mode %q (want invalid-json, drop-fields or wrong-types)", p, ft.Corrupt)
		}
		if ft.Fraction < 0 || ft.Fraction > 1 {
			add("%s.fault.fraction: must be in [0,1], got %v", p, ft.Fraction)
		}
		if ft.Bytes < 0 {
			add("%s.fault.bytes: must be >= 0, got %d", p, ft.Bytes)
		}

		ex := e.Expect
		if ex.MaxCalls != nil {
			toolRef(p+".expect.max_calls.tool", ex.MaxCalls.Tool)
			if ex.MaxCalls.Count < 0 {
				add("%s.expect.max_calls.count: must be >= 0", p)
			}
		}
		for j, t := range ex.MustCall {
			toolRef(fmt.Sprintf("%s.expect.must_call[%d]", p, j), t)
		}
		for j, t := range ex.MustNotCall {
			toolRef(fmt.Sprintf("%s.expect.must_not_call[%d]", p, j), t)
		}
		if ex.StopsWithin != nil && *ex.StopsWithin < 0 {
			add("%s.expect.stops_within: must be >= 0", p)
		}
		checkRegex(p+".expect.output_matches", ex.OutputMatches)
		checkRegex(p+".expect.output_not_matches", ex.OutputNotMatches)
	}
	return errors.Join(errs...)
}
