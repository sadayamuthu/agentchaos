package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
)

var binDir string

func TestMain(m *testing.M) { os.Exit(testMain(m)) }

func testMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "agentchaos-examples-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	binDir = dir
	for name, pkg := range map[string]string{
		"agentchaos":    "../cmd/agentchaos",
		"crm-server":    "./crm-server",
		"orders-server": "./orders-server",
	} {
		out, err := exec.Command("go", "build", "-o", filepath.Join(dir, name), pkg).CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s", name, err, out)
			return 1
		}
	}
	return m.Run()
}

func TestExampleSuiteValidates(t *testing.T) {
	f, err := experiment.Load("support-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if f.Runs != 3 || len(f.Experiments) != 4 {
		t.Fatalf("runs=%d experiments=%d", f.Runs, len(f.Experiments))
	}
}

// connect starts the server through `agentchaos proxy` with the named
// experiment (whose fault must not match the calls the test makes).
func connect(t *testing.T, expName, server string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "agentchaos"), "proxy",
		"--experiment", "support-agent.yaml", "--name", expName, "--server", server,
		"--", filepath.Join(binDir, server+"-server"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	sess, err := c.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

func call(t *testing.T, s *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func sameJSON(t *testing.T, got any, want string) {
	t.Helper()
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(g) != fmt.Sprint(w) {
		t.Fatalf("got %s, want %s", b, want)
	}
}

func TestExampleServersRespond(t *testing.T) {
	t.Run("crm lookup", func(t *testing.T) {
		// The fault targets orders.refund, so crm calls pass through.
		s := connect(t, "orders.refund error", "crm")
		res := call(t, s, "lookup", map[string]any{"customer_id": 4411})
		if res.IsError {
			t.Fatalf("tool error: %+v", res.Content)
		}
		sameJSON(t, res.StructuredContent, `{"customer_id":4411,"name":"Ada Example","plan":"enterprise","seats":40,"email":"ada@example.com"}`)
		if res := call(t, s, "lookup", map[string]any{"customer_id": 1}); !res.IsError {
			t.Fatal("unknown customer: want IsError")
		}
	})
	t.Run("orders", func(t *testing.T) {
		// The fault targets crm.lookup, so orders calls pass through.
		s := connect(t, "crm.lookup timeout", "orders")
		res := call(t, s, "refund", map[string]any{"order_id": "ord_1001", "amount": 25.5})
		if res.IsError {
			t.Fatalf("tool error: %+v", res.Content)
		}
		sameJSON(t, res.StructuredContent, `{"refund_id":"rf_1","status":"refunded"}`)
		if res := call(t, s, "refund", map[string]any{"order_id": "nope", "amount": 1}); !res.IsError {
			t.Fatal("unknown order: want IsError")
		}
		if res := call(t, s, "get", map[string]any{"order_id": "ord_1001"}); res.IsError || res.StructuredContent == nil {
			t.Fatalf("get: %+v", res)
		}
		if res := call(t, s, "cancel", map[string]any{"order_id": "nope"}); !res.IsError {
			t.Fatal("cancel unknown: want IsError")
		}
	})
}
