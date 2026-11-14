package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/permissions"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/tools"
	"github.com/mr-daaku/opentitam/mockserver"
)

// defaultReg builds the standard tool registry for tests.
func defaultReg() *tools.Registry {
	return tools.DefaultRegistry("")
}

// TestAgentToolLoop runs a full round trip against the mock server:
// user prompt → model requests tool → tool executes → model replies.
func TestAgentToolLoop(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(target, []byte("secret-content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Script: turn 1 → tool call (read); turn 2 → final text.
	srv := mockserver.New(
		mockserver.Turn{ToolCall: &mockserver.ToolCall{
			ID: "call_read", Name: "read",
			Arguments: `{"path":"` + filepath.ToSlash(target) + `"}`,
		}},
		mockserver.Turn{Text: "The file contains secret-content."},
	)
	defer srv.Close()

	client := provider.NewClient("mock", srv.URL(), "")
	reg := defaultReg()
	perms := permissions.New(nil) // base policy: read allowed

	ag := New(client, "mock-small", reg, perms, dir)

	var events []Event
	var toolResults []string
	for ev := range ag.Run(context.Background(), nil, "read hello.txt") {
		events = append(events, ev)
		if ev.Kind == "tool_end" {
			toolResults = append(toolResults, ev.Result)
		}
	}

	// The tool must have executed and read the real file.
	if len(toolResults) != 1 {
		t.Fatalf("tool_end events = %d, want 1; events=%+v", len(toolResults), kinds(events))
	}
	if !strings.Contains(toolResults[0], "secret-content") {
		t.Errorf("tool result = %q", toolResults[0])
	}

	// Final done event carries the full message list incl. tool result.
	var done *Event
	for i := range events {
		if events[i].Kind == "done" {
			done = &events[i]
		}
	}
	if done == nil {
		t.Fatalf("no done event; kinds=%v", kinds(events))
	}
	var msgs []provider.Message
	if err := json.Unmarshal([]byte(done.Result), &msgs); err != nil {
		t.Fatalf("done payload: %v", err)
	}
	roles := []string{}
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	// user → assistant(tool_call) → tool → assistant(final)
	if len(msgs) != 4 {
		t.Errorf("messages = %d (%v), want 4", len(msgs), roles)
	}
	if msgs[len(msgs)-1].Role != "assistant" || !strings.Contains(msgs[len(msgs)-1].Content, "secret-content") {
		t.Errorf("final message = %+v", msgs[len(msgs)-1])
	}
	// Server saw the tool result come back.
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("server requests = %d, want 2", len(reqs))
	}
	if !mockserver.HasToolResult(reqs[1].Messages) {
		t.Errorf("second request missing tool result message")
	}
}

// TestAgentPermissionDeny: deny rule blocks the tool without executing it.
func TestAgentPermissionDeny(t *testing.T) {
	srv := mockserver.New(
		mockserver.Turn{ToolCall: &mockserver.ToolCall{
			ID: "call_shell", Name: "shell", Arguments: `{"command":"echo pwned"}`,
		}},
		mockserver.Turn{Text: "ok"},
	)
	defer srv.Close()

	client := provider.NewClient("mock", srv.URL(), "")
	perms := permissions.New([]config.Permission{
		{Action: "shell", Resource: "*", Effect: "deny"},
	})

	ag := New(client, "mock-small", defaultReg(), perms, t.TempDir())

	var toolErr error
	var toolResult string
	for ev := range ag.Run(context.Background(), nil, "run something") {
		if ev.Kind == "tool_end" {
			toolErr = ev.Err
			toolResult = ev.Result
		}
	}
	if toolErr == nil {
		t.Fatalf("expected permission denial on tool_end")
	}
	if !strings.Contains(toolResult, "denied") {
		t.Errorf("result = %q", toolResult)
	}
}

// TestAgentAskApprovalReject: ask effect + user rejects → tool not executed.
func TestAgentAskApprovalReject(t *testing.T) {
	srv := mockserver.New(
		mockserver.Turn{ToolCall: &mockserver.ToolCall{
			ID: "c1", Name: "shell", Arguments: `{"command":"echo hi"}`,
		}},
		mockserver.Turn{Text: "ok"},
	)
	defer srv.Close()

	client := provider.NewClient("mock", srv.URL(), "")
	perms := permissions.New([]config.Permission{
		{Action: "shell", Resource: "*", Effect: "ask"},
	})
	ag := New(client, "mock-small", defaultReg(), perms, t.TempDir())

	asked := false
	ag.SetApprover(func(action, resource, command string) string {
		asked = true
		if action != "shell" || !strings.Contains(command, "echo hi") {
			t.Errorf("approver got action=%q resource=%q command=%q", action, resource, command)
		}
		return "reject"
	})

	var toolErr error
	for ev := range ag.Run(context.Background(), nil, "run") {
		if ev.Kind == "tool_end" {
			toolErr = ev.Err
		}
	}
	if !asked {
		t.Fatalf("approver was never called")
	}
	if toolErr == nil {
		t.Fatalf("expected rejection error")
	}
	if !strings.Contains(toolErr.Error(), "rejected") {
		t.Errorf("err = %v", toolErr)
	}
}

func kinds(evs []Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}
