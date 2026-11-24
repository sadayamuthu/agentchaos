package pilotdata

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fixture = "../../cmd/analyze/testdata/data/model-a/orders-refund-error/1"

func TestToolCallsPairsResponsesAndKeepsOnlyArguments(t *testing.T) {
	calls, err := ToolCalls(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Name != "orders.get" || calls[1].Name != "orders.refund" {
		t.Fatalf("calls = %+v", calls)
	}
	if !calls[1].IsError || calls[1].Response != "internal server error (500)" || !calls[1].Answered {
		t.Errorf("refund call = %+v", calls[1])
	}
	if got := string(calls[0].Arguments); strings.Contains(got, "claude") || !strings.Contains(got, "ord_1001") {
		t.Errorf("arguments should be only the tool arguments, got %s", got)
	}
}

func TestToolSequenceOrdersAcrossServers(t *testing.T) {
	got, err := ToolSequence(filepath.Join("../../cmd/analyze/testdata/data/model-a/orders-refund-error-control/1"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"crm.lookup", "orders.get", "orders.refund"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestRunDirIgnoresAbsolutePrefix(t *testing.T) {
	if got := RunDir("/r", "/Users/x/data/slug/3"); got != filepath.Join("/r", "slug", "3") {
		t.Errorf("got %s", got)
	}
	if RunDir("/r", "3") != "" {
		t.Error("a dir with fewer than two components is unusable")
	}
}
