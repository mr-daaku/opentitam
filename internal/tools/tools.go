// Package tools implements OpenTitam's coding tools:
// read, write, edit, glob, grep, shell.
//
// Every tool is exposed with a JSON schema for OpenAI-style function calling
// and an Execute method. Permission checks happen in the agent before Execute.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is one executable tool with its function-calling schema.
type Tool struct {
	Name        string
	Description string
	// Schema is the JSON schema for parameters (OpenAI function calling).
	Schema json.RawMessage
	// Execute runs the tool and returns the string result for the model.
	Execute func(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry maps tool name → Tool.
type Registry struct {
	tools map[string]*Tool
	order []string
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]*Tool{}}
}

func (r *Registry) Register(t *Tool) {
	if _, ok := r.tools[t.Name]; !ok {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (*Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns tools in registration order.
func (r *Registry) All() []*Tool {
	out := make([]*Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}
	return out
}

// DefaultRegistry registers the six built-in coding tools.
func DefaultRegistry(shell string) *Registry {
	r := NewRegistry()
	r.Register(readTool())
	r.Register(writeTool())
	r.Register(editTool())
	r.Register(globTool())
	r.Register(grepTool())
	r.Register(shellTool(shell))
	return r
}

// funcSchema is a helper to build an OpenAI function parameters schema.
func funcSchema(desc string, props map[string]any, required []string) json.RawMessage {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	b, _ := json.Marshal(s)
	_ = desc
	return b
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// ErrTool is returned with a user-facing message when a tool fails.
type ErrTool struct{ Msg string }

func (e *ErrTool) Error() string { return e.Msg }

func toolErrf(format string, a ...any) error {
	return &ErrTool{Msg: fmt.Sprintf(format, a...)}
}
