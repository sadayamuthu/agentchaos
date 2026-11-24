// Command testserver is a stdio MCP server with two tools, for AgentChaos
// tests: lookup returns {"plan":"enterprise","seats":40} and other returns ok.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type empty struct{}

func textTool(text string) mcp.ToolHandlerFor[empty, any] {
	return func(context.Context, *mcp.CallToolRequest, empty) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	}
}

func main() {
	s := mcp.NewServer(&mcp.Implementation{Name: "testserver", Version: "0.0.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "lookup", Description: "look up an account"}, textTool(`{"plan":"enterprise","seats":40}`))
	mcp.AddTool(s, &mcp.Tool{Name: "other", Description: "another tool"}, textTool("ok"))
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "testserver:", err)
		os.Exit(1)
	}
}
