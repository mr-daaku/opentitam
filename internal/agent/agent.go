// Package agent implements the OpenTitam agent loop:
// model ⇄ tools until the model stops calling tools.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mr-daaku/opentitam/internal/permissions"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/tools"
)

// Event is emitted by the agent loop for the UI (TUI or headless run).
type Event struct {
	Kind string // "text_delta" | "text_done" | "tool_start" | "tool_end" | "error" | "done"
	Text string
	// Tool fields.
	ToolName string
	ToolArgs json.RawMessage
	ToolID   string
	Result   string
	Err      error
	// Usage fields on "done".
	InputTokens  int
	OutputTokens int
}

// ApprovalFunc asks the user about a permission check.
// Returns "allow", "allow_always", or "reject".
type ApprovalFunc func(action, resource, command string) string

// Agent runs the loop.
type Agent struct {
	Client     *provider.Client
	Model      string
	Tools      *tools.Registry
	Perms      *permissions.Engine
	Cwd        string
	System     string
	Approve    ApprovalFunc
	// AlwaysAllow collects "allow always" patterns for the session.
	AlwaysAllow []permissionsRule
	maxRounds   int
}

// permissionsRule avoids importing config here for the always-allow list.
type permissionsRule struct {
	Action   string
	Resource string
}

// New creates an agent with defaults.
func New(client *provider.Client, model string, reg *tools.Registry, perms *permissions.Engine, cwd string) *Agent {
	return &Agent{
		Client:  client,
		Model:   model,
		Tools:   reg,
		Perms:   perms,
		Cwd:     cwd,
		System:  SystemPrompt(cwd),
		maxRounds: 50,
	}
}

// SetApprover wires the UI approval callback.
func (a *Agent) SetApprover(fn ApprovalFunc) { a.Approve = fn }

// Run executes one user turn. It emits events on the returned channel.
// msgs must be the full conversation so far (without the new user message);
// the new user prompt is appended here. The updated full message list is
// sent as the final Event's Result field (JSON) so the caller can persist it.
func (a *Agent) Run(ctx context.Context, history []provider.Message, userPrompt string) <-chan Event {
	out := make(chan Event, 128)
	go func() {
		defer close(out)
		msgs := append([]provider.Message{}, history...)
		msgs = append(msgs, provider.Message{Role: "user", Content: userPrompt})
		msgs = a.trim(msgs)

		toolDefs := a.toolDefs()

		for round := 0; round < a.maxRounds; round++ {
			if ctx.Err() != nil {
				out <- Event{Kind: "error", Err: ctx.Err()}
				return
			}
			assistant, deltas, err := a.streamOnce(ctx, msgs, toolDefs, out)
			if err != nil {
				out <- Event{Kind: "error", Err: err}
				return
			}
			_ = deltas
			msgs = append(msgs, assistant)

			if len(assistant.ToolCalls) == 0 {
				// Finished.
				out <- Event{Kind: "done", Result: marshalMessages(msgs)}
				return
			}

			// Execute each tool call.
			for _, tc := range assistant.ToolCalls {
				result := a.execTool(ctx, tc, out)
				msgs = append(msgs, provider.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})
			}
		}
		out <- Event{Kind: "error", Err: fmt.Errorf("max tool rounds (%d) exceeded", a.maxRounds)}
	}()
	return out
}

// streamOnce does one streaming request, forwarding text deltas to out.
func (a *Agent) streamOnce(ctx context.Context, msgs []provider.Message, toolDefs []provider.ToolDef, out chan<- Event) (provider.Message, string, error) {
	ch, err := a.Client.ChatStream(ctx, provider.ChatRequest{
		Model:    a.Model,
		Messages: append([]provider.Message{{Role: "system", Content: a.System}}, msgs...),
		Tools:    toolDefs,
	})
	if err != nil {
		return provider.Message{}, "", err
	}
	var text strings.Builder
	var toolCalls []provider.ToolCall
	// Index-keyed accumulator for streamed tool call fragments.
	acc := map[int]*provider.ToolCall{}

	for ev := range ch {
		if ev.Err != nil {
			return provider.Message{}, text.String(), ev.Err
		}
		if ev.Done {
			break
		}
		if ev.DeltaText != "" {
			text.WriteString(ev.DeltaText)
			out <- Event{Kind: "text_delta", Text: ev.DeltaText}
		}
		if d := ev.ToolCallDelta; d != nil {
			tc, ok := acc[d.Index]
			if !ok {
				tc = &provider.ToolCall{ID: fmt.Sprintf("call_%d_%d", len(acc), d.Index), Type: "function"}
				acc[d.Index] = tc
			}
			if d.ID != "" {
				tc.ID = d.ID
			}
			if d.Name != "" {
				tc.Function.Name = d.Name
			}
			tc.Function.Arguments += d.Arguments
		}
	}
	// Materialize tool calls sorted by index.
	for i := 0; ; i++ {
		tc, ok := acc[i]
		if !ok {
			break
		}
		toolCalls = append(toolCalls, *tc)
	}
	assistant := provider.Message{Role: "assistant", Content: text.String()}
	if len(toolCalls) > 0 {
		assistant.ToolCalls = toolCalls
		assistant.Content = "" // OpenAI: content optional when tool_calls present
	}
	out <- Event{Kind: "text_done", Text: text.String()}
	return assistant, text.String(), nil
}

// execTool runs one tool call with permission checks.
func (a *Agent) execTool(ctx context.Context, tc provider.ToolCall, out chan<- Event) string {
	out <- Event{Kind: "tool_start", ToolName: tc.Function.Name, ToolArgs: json.RawMessage(tc.Function.Arguments), ToolID: tc.ID}

	t, ok := a.Tools.Get(tc.Function.Name)
	if !ok {
		msg := fmt.Sprintf("Unknown tool: %s", tc.Function.Name)
		out <- Event{Kind: "tool_end", ToolName: tc.Function.Name, ToolID: tc.ID, Result: msg, Err: errors.New(msg)}
		return msg
	}

	// Permission check (maps tool name → permission action, consults
	// session always-allow rules, then the engine).
	res := resourceFor(tc.Function.Name, tc.Function.Arguments, a.Cwd)
	effect := a.checkPermission(tc.Function.Name, tc.Function.Arguments)

	if effect == "deny" {
		msg := fmt.Sprintf("Permission denied for %s on %s", tc.Function.Name, res)
		out <- Event{Kind: "tool_end", ToolName: tc.Function.Name, ToolID: tc.ID, Result: msg, Err: fmt.Errorf("denied")}
		return msg
	}
	if effect == "ask" && a.Approve != nil {
		decision := a.Approve(permAction(tc.Function.Name), res, extractCommand(tc.Function.Arguments))
		switch decision {
		case "reject":
			msg := "User rejected this operation."
			out <- Event{Kind: "tool_end", ToolName: tc.Function.Name, ToolID: tc.ID, Result: msg, Err: fmt.Errorf("rejected")}
			return msg
		case "allow_always":
			a.AlwaysAllow = append(a.AlwaysAllow, permissionsRule{Action: permAction(tc.Function.Name), Resource: savedPattern(res)})
		}
	}

	var args json.RawMessage = json.RawMessage(tc.Function.Arguments)
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	result, err := t.Execute(ctx, args)
	if err != nil {
		msg := fmt.Sprintf("Error: %v", err)
		out <- Event{Kind: "tool_end", ToolName: tc.Function.Name, ToolID: tc.ID, Result: msg, Err: err}
		return msg
	}
	out <- Event{Kind: "tool_end", ToolName: tc.Function.Name, ToolID: tc.ID, Result: result}
	return result
}

// checkPermission evaluates the engine for a tool call (after session
// always-allow additions are folded in by the engine caller).
func (a *Agent) checkPermission(toolName, rawArgs string) string {
	action := permAction(toolName)
	res := resourceFor(toolName, rawArgs, a.Cwd)
	// Session-level always-allow rules first.
	for _, r := range a.AlwaysAllow {
		if r.Action == action && strings.Contains(res, strings.TrimSuffix(r.Resource, "*")) {
			return "allow"
		}
	}
	return a.Perms.Check(action, res)
}

func permAction(toolName string) string {
	switch toolName {
	case "read":
		return "read"
	case "shell":
		return "shell"
	case "write", "edit":
		return "edit"
	case "glob":
		return "glob"
	case "grep":
		return "grep"
	default:
		return toolName
	}
}

// resourceFor extracts the permission resource from tool arguments.
func resourceFor(toolName, rawArgs, cwd string) string {
	var m map[string]any
	_ = json.Unmarshal([]byte(rawArgs), &m)
	switch toolName {
	case "read", "write", "edit":
		p, _ := m["path"].(string)
		return absResource(p, cwd)
	case "shell":
		cmd, _ := m["command"].(string)
		return cmd
	case "glob":
		pat, _ := m["pattern"].(string)
		return pat
	case "grep":
		pat, _ := m["pattern"].(string)
		return pat
	default:
		return "*"
	}
}

func absResource(p, cwd string) string {
	if p == "" {
		return p
	}
	if filepath.IsAbs(p) {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(filepath.Join(cwd, p))
}

func extractCommand(rawArgs string) string {
	var m map[string]any
	if json.Unmarshal([]byte(rawArgs), &m) != nil {
		return ""
	}
	c, _ := m["command"].(string)
	return c
}

func savedPattern(res string) string {
	// Save a prefix pattern for shell: first two tokens + " *"
	parts := strings.Fields(res)
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1] + " *"
	}
	if len(parts) == 1 {
		return parts[0] + " *"
	}
	return "*"
}

func (a *Agent) toolDefs() []provider.ToolDef {
	var defs []provider.ToolDef
	for _, t := range a.Tools.All() {
		defs = append(defs, provider.ToolDef{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		})
	}
	return defs
}

// trim keeps the message list within a rough token budget by dropping
// oldest non-system messages first (never the newest user message).
func (a *Agent) trim(msgs []provider.Message) []provider.Message {
	const approxTokenBudget = 120_000
	for estimateTokens(msgs) > approxTokenBudget && len(msgs) > 3 {
		// Drop the oldest message (but keep at least the first user message).
		if msgs[0].Role == "user" && len(msgs) > 1 {
			msgs = msgs[1:]
		} else {
			msgs = msgs[1:]
		}
	}
	return msgs
}

func estimateTokens(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += utf8.RuneCountInString(m.Content)/4 + 8
		for _, tc := range m.ToolCalls {
			total += utf8.RuneCountInString(tc.Function.Arguments)/4 + 16
		}
	}
	return total
}

func marshalMessages(msgs []provider.Message) string {
	b, _ := json.Marshal(msgs)
	return string(b)
}

// LoadInstructions reads AGENTS.md / OPENCODE.md / OPEN TITAM.md style files
// from cwd and merges them into the system prompt.
func LoadInstructions(cwd string) string {
	var parts []string
	for _, name := range []string{"AGENTS.md", "OPEN TITAM.md", "OPENTITAM.md", "CLAUDE.md"} {
		p := filepath.Join(cwd, name)
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			parts = append(parts, fmt.Sprintf("# Instructions from %s\n%s", name, strings.TrimSpace(string(b))))
		}
	}
	return strings.Join(parts, "\n\n")
}

