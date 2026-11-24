// Package pilotdata reads pilot output: report.json files, run directories
// and the switchyard traces inside them. It is shared by the analyze and
// labelsheet tools.
package pilotdata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sadayamuthu/switchyard"
)

// Check is one expectation result of a run.
type Check struct {
	Name string `json:"name"`
	Pass bool   `json:"pass"`
}

// Run mirrors the parts of a report.json run the tools use.
type Run struct {
	Index     int     `json:"index"`
	Status    string  `json:"status"`
	Dir       string  `json:"dir"`
	ToolCalls int     `json:"tool_calls"`
	Checks    []Check `json:"checks"`
}

// Experiment is one experiment's runs in a report.
type Experiment struct {
	Name string `json:"name"`
	Runs []Run  `json:"runs"`
}

// Report mirrors the parts of report.json (version 1) the tools use.
type Report struct {
	Version     int          `json:"version"`
	Experiments []Experiment `json:"experiments"`
}

// Model is one DATA/<name>/report.json together with its directory.
type Model struct {
	Name   string
	Dir    string
	Report Report
}

// LoadModels reads DATA/<model>/report.json for every model directory, in
// directory-name order. Directories without a report.json are skipped.
func LoadModels(data string) ([]*Model, error) {
	entries, err := os.ReadDir(data)
	if err != nil {
		return nil, err
	}
	var models []*Model
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(data, e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "report.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		m := &Model{Name: e.Name(), Dir: dir}
		if err := json.Unmarshal(raw, &m.Report); err != nil {
			return nil, fmt.Errorf("%s/report.json: %w", e.Name(), err)
		}
		if m.Report.Version != 1 {
			return nil, fmt.Errorf("%s/report.json: unsupported version %d", e.Name(), m.Report.Version)
		}
		models = append(models, m)
	}
	return models, nil
}

// RunDir locates a run directory relative to the report's own directory,
// using the last two components (<slug>/<index>) of the recorded dir. It
// never opens the recorded absolute path.
func RunDir(reportDir, recorded string) string {
	parts := strings.FieldsFunc(recorded, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) < 2 {
		return ""
	}
	return filepath.Join(reportDir, parts[len(parts)-2], parts[len(parts)-1])
}

// Call is one tools/call request with its response, if one was traced.
type Call struct {
	Name      string // "server.tool", prefixed by the hop
	Time      time.Time
	Arguments json.RawMessage // the tool arguments only, never other params
	Response  string          // result content text, or the error message
	IsError   bool
	Answered  bool
}

// ToolCalls returns the tools/call requests of all trace-*.jsonl files of a
// run directory in time order. Events carry no cross-server sequence number,
// so ties keep file then line order.
func ToolCalls(dir string) ([]Call, error) {
	files, err := filepath.Glob(filepath.Join(dir, "trace-*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no trace files in %s", dir)
	}
	sort.Strings(files)
	var calls []Call
	for _, f := range files {
		fc, err := readTrace(f)
		if err != nil {
			return nil, err
		}
		calls = append(calls, fc...)
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].Time.Before(calls[j].Time) })
	return calls, nil
}

func readTrace(path string) ([]Call, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var calls []Call
	pending := map[string]int{} // session+id -> index in calls
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev switchyard.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if ev.Method != "tools/call" {
			continue
		}
		key := ev.Session + "\x00" + ev.ID
		switch ev.Kind {
		case switchyard.KindRequest:
			var p struct {
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(ev.Params, &p)
			pending[key] = len(calls)
			calls = append(calls, Call{Name: ev.Hop + "." + ev.Tool, Time: ev.Time, Arguments: p.Arguments})
		case switchyard.KindResponse:
			i, ok := pending[key]
			if !ok {
				continue
			}
			delete(pending, key)
			calls[i].Answered = true
			calls[i].Response, calls[i].IsError = responseText(ev)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return calls, nil
}

func responseText(ev switchyard.Event) (text string, isErr bool) {
	if ev.Error != nil {
		return ev.Error.Message, true
	}
	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(ev.Result, &r) != nil {
		return "", false
	}
	var parts []string
	for _, c := range r.Content {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, "\n"), r.IsError
}

// ToolSequence returns the "server.tool" names of a run's tool calls in time
// order.
func ToolSequence(dir string) ([]string, error) {
	calls, err := ToolCalls(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	return names, nil
}
