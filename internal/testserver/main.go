// Command testserver is a stdio MCP server with two tools, for AgentChaos
// tests: lookup returns {"plan":"enterprise","seats":40} and other returns ok.
// If TESTSERVER_PIDFILE is set, it writes its pid to that file at start.
// If TESTSERVER_TYPED is set, both tools are typed: they declare an
// outputSchema and return structuredContent, lookup {"plan","seats"} and
// other {"status":"ok"}.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type empty struct{}

func textTool(text string) mcp.ToolHandlerFor[empty, any] {
	return func(context.Context, *mcp.CallToolRequest, empty) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	}
}

type account struct {
	Plan  string `json:"plan"`
	Seats int    `json:"seats"`
}

type status struct {
	Status string `json:"status"`
}

func typedTool[Out any](out Out) mcp.ToolHandlerFor[empty, Out] {
	return func(context.Context, *mcp.CallToolRequest, empty) (*mcp.CallToolResult, Out, error) {
		return nil, out, nil
	}
}

func main() {
	if p := os.Getenv("TESTSERVER_PIDFILE"); p != "" {
		if err := os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "testserver:", err)
			os.Exit(1)
		}
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "testserver", Version: "0.0.0"}, nil)
	if os.Getenv("TESTSERVER_TYPED") != "" {
		mcp.AddTool(s, &mcp.Tool{Name: "lookup", Description: "look up an account"}, typedTool(account{Plan: "enterprise", Seats: 40}))
		mcp.AddTool(s, &mcp.Tool{Name: "other", Description: "another tool"}, typedTool(status{Status: "ok"}))
	} else {
		mcp.AddTool(s, &mcp.Tool{Name: "lookup", Description: "look up an account"}, textTool(`{"plan":"enterprise","seats":40}`))
		mcp.AddTool(s, &mcp.Tool{Name: "other", Description: "another tool"}, textTool("ok"))
	}
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "testserver:", err)
		os.Exit(1)
	}
}
