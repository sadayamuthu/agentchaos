// Command orders-server is a tiny fake order system as a stdio MCP server,
// for the AgentChaos examples. Its data is in memory and has no effect
// beyond the process.
package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type order struct {
	OrderID    string  `json:"order_id"`
	CustomerID int     `json:"customer_id"`
	Total      float64 `json:"total"`
	Status     string  `json:"status"`
}

type store struct {
	mu      sync.Mutex
	orders  map[string]*order
	refunds int
}

func newStore() *store {
	return &store{orders: map[string]*order{
		"ord_1001": {OrderID: "ord_1001", CustomerID: 4411, Total: 129.99, Status: "shipped"},
		"ord_1002": {OrderID: "ord_1002", CustomerID: 4411, Total: 59.5, Status: "processing"},
	}}
}

func (s *store) find(id string) (*order, error) {
	o, ok := s.orders[id]
	if !ok {
		return nil, fmt.Errorf("order %q not found", id)
	}
	return o, nil
}

type orderIn struct {
	OrderID string `json:"order_id" jsonschema:"order id, for example ord_1001"`
}

type refundIn struct {
	OrderID string  `json:"order_id" jsonschema:"id of the order to refund, for example ord_1001"`
	Amount  float64 `json:"amount" jsonschema:"amount to refund in dollars; must be positive and no more than the order total"`
}

type refundOut struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"`
}

func (s *store) get(_ context.Context, _ *mcp.CallToolRequest, in orderIn) (*mcp.CallToolResult, order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.find(in.OrderID)
	if err != nil {
		return nil, order{}, err
	}
	return nil, *o, nil
}

func (s *store) refund(_ context.Context, _ *mcp.CallToolRequest, in refundIn) (*mcp.CallToolResult, refundOut, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.find(in.OrderID)
	if err != nil {
		return nil, refundOut{}, err
	}
	if in.Amount <= 0 || in.Amount > o.Total {
		return nil, refundOut{}, fmt.Errorf("invalid refund amount %v for order total %v", in.Amount, o.Total)
	}
	if o.Status == "refunded" || o.Status == "cancelled" {
		return nil, refundOut{}, fmt.Errorf("order %q is already %s", o.OrderID, o.Status)
	}
	s.refunds++
	o.Status = "refunded"
	return nil, refundOut{RefundID: fmt.Sprintf("rf_%d", s.refunds), Status: "refunded"}, nil
}

func (s *store) cancel(_ context.Context, _ *mcp.CallToolRequest, in orderIn) (*mcp.CallToolResult, order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.find(in.OrderID)
	if err != nil {
		return nil, order{}, err
	}
	if o.Status != "processing" {
		return nil, order{}, fmt.Errorf("order %q is %s and can no longer be cancelled", o.OrderID, o.Status)
	}
	o.Status = "cancelled"
	return nil, *o, nil
}

func main() {
	st := newStore()
	s := mcp.NewServer(&mcp.Implementation{Name: "orders-server", Version: "0.0.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get",
		Description: "Get an order by order id. Returns its customer id, total and status. Read-only.",
	}, st.get)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "refund",
		Description: "Refund money to the customer for an order. Takes the order id and the dollar amount to refund. Use this when the user asks for a refund. Does not cancel the order.",
	}, st.refund)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "cancel",
		Description: "Cancel an order that has not shipped yet. Only use this when the user asks to cancel an order, never as a substitute for a refund.",
	}, st.cancel)
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "orders-server:", err)
		os.Exit(1)
	}
}
