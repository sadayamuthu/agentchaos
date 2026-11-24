// Command crm-server is a tiny fake CRM as a stdio MCP server, for the
// AgentChaos examples. Its data is in memory and fixed.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type lookupIn struct {
	CustomerID int `json:"customer_id" jsonschema:"numeric customer id, for example 4411"`
}

type customer struct {
	CustomerID int    `json:"customer_id"`
	Name       string `json:"name"`
	Plan       string `json:"plan"`
	Seats      int    `json:"seats"`
	Email      string `json:"email"`
}

var customers = map[int]customer{
	4411: {CustomerID: 4411, Name: "Ada Example", Plan: "enterprise", Seats: 40, Email: "ada@example.com"},
}

func lookup(_ context.Context, _ *mcp.CallToolRequest, in lookupIn) (*mcp.CallToolResult, customer, error) {
	c, ok := customers[in.CustomerID]
	if !ok {
		return nil, customer{}, fmt.Errorf("customer %d not found", in.CustomerID)
	}
	return nil, c, nil
}

func main() {
	s := mcp.NewServer(&mcp.Implementation{Name: "crm-server", Version: "0.0.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "lookup",
		Description: "Look up a customer in the CRM by numeric customer id. Returns the customer's name, subscription plan, number of seats and contact email. Use this to answer questions about a customer's plan or account.",
	}, lookup)
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "crm-server:", err)
		os.Exit(1)
	}
}
