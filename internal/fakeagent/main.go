// Command fakeagent is a scripted MCP client standing in for an AI agent in
// AgentChaos tests. It connects to one server from an MCP config, calls one
// tool up to --max-attempts times until it gets a non-error result, and then
// reports, stays silent or hangs as --on-failure says.
//
// Usage:
//
//	fakeagent --mcp-config PATH [--prompt S] [--print-prompt] [--call server.tool]
//	    [--args JSON] [--max-attempts N] [--backoff none|exp] [--base DURATION]
//	    [--on-failure report|silent|hang]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type config struct {
	MCPServers map[string]struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	} `json:"mcpServers"`
}

func main() {
	fs := flag.NewFlagSet("fakeagent", flag.ContinueOnError)
	mcpConfig := fs.String("mcp-config", "", "MCP config `PATH`")
	prompt := fs.String("prompt", "", "the task prompt")
	printPrompt := fs.Bool("print-prompt", false, "echo the prompt to stdout")
	call := fs.String("call", "", "tool to call, as `server.tool`")
	argsJSON := fs.String("args", "{}", "tool arguments as a JSON object")
	maxAttempts := fs.Int("max-attempts", 1, "maximum number of calls")
	backoff := fs.String("backoff", "none", "sleep between attempts: none or exp")
	base := fs.Duration("base", 100*time.Millisecond, "first backoff sleep; doubled each retry")
	onFailure := fs.String("on-failure", "report", "after the last failure: report, silent or hang")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(*argsJSON), &args); err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent: --args:", err)
		os.Exit(2)
	}
	switch {
	case *backoff != "none" && *backoff != "exp":
		fmt.Fprintf(os.Stderr, "fakeagent: --backoff: want none or exp, got %q\n", *backoff)
		os.Exit(2)
	case *onFailure != "report" && *onFailure != "silent" && *onFailure != "hang":
		fmt.Fprintf(os.Stderr, "fakeagent: --on-failure: want report, silent or hang, got %q\n", *onFailure)
		os.Exit(2)
	case *maxAttempts < 1:
		fmt.Fprintln(os.Stderr, "fakeagent: --max-attempts must be >= 1")
		os.Exit(2)
	}

	if *printPrompt {
		fmt.Println(*prompt)
	}
	if *call == "" {
		return
	}
	text, err := run(*mcpConfig, *call, args, *maxAttempts, *backoff, *base, *onFailure == "hang")
	if err == nil {
		fmt.Println(text)
		return
	}
	switch *onFailure {
	case "report":
		fmt.Printf("unable to complete: %v\n", err)
	case "hang":
		hang()
	}
}

// hang blocks forever. A sleeping loop, unlike select {}, is never mistaken
// for a deadlock by the runtime.
func hang() {
	for {
		time.Sleep(time.Hour)
	}
}

// run connects to the server named in call and calls its tool, retrying
// failures. It returns the result text of the first success, or the last
// failure. With hangOnFailure, a final failure blocks forever with the
// session still open, as an agent stuck mid-task would.
func run(configPath, call string, args map[string]any, maxAttempts int, backoff string, base time.Duration, hangOnFailure bool) (string, error) {
	server, tool, ok := strings.Cut(call, ".")
	if !ok {
		return "", fmt.Errorf("--call %q: want server.tool", call)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("%s: %w", configPath, err)
	}
	srv, ok := cfg.MCPServers[server]
	if !ok {
		return "", fmt.Errorf("server %q not in %s", server, configPath)
	}
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Env = os.Environ()
	for k, v := range srv.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stderr = os.Stderr

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "fakeagent", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return "", err
	}
	defer cs.Close()

	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && backoff == "exp" {
			time.Sleep(base << (attempt - 2))
		}
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		switch {
		case err != nil:
			last = err
		case r.IsError:
			last = errors.New(resultText(r))
		default:
			return resultText(r), nil
		}
	}
	if hangOnFailure {
		hang()
	}
	return "", last
}

func resultText(r *mcp.CallToolResult) string {
	var parts []string
	for _, c := range r.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, "\n")
}
