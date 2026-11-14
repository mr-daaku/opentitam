package provider

import (
	"context"
	"testing"

	"github.com/mr-daaku/opentitam/mockserver"
)

// TestDynamicModelsLoad proves there is NO built-in catalog: models come
// only from GET {baseURL}/models.
func TestDynamicModelsLoad(t *testing.T) {
	srv := mockserver.New()
	defer srv.Close()

	c := NewClient("mock", srv.URL(), "test-key")
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2 (dynamic)", len(models))
	}
	if models[0].ID != "mock-small" || models[1].ID != "mock-large" {
		t.Errorf("ids = %s, %s", models[0].ID, models[1].ID)
	}
}

func TestListModelsBadURL(t *testing.T) {
	c := NewClient("bad", "http://127.0.0.1:1", "")
	if _, err := c.ListModels(context.Background()); err == nil {
		t.Fatalf("expected error for unreachable endpoint")
	}
}

// TestStreamText verifies SSE delta merging.
func TestStreamText(t *testing.T) {
	srv := mockserver.New(mockserver.Turn{Text: "Hello World"})
	defer srv.Close()

	c := NewClient("mock", srv.URL(), "")
	ch, err := c.ChatStream(context.Background(), ChatRequest{
		Model:    "mock-small",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var got string
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream err: %v", ev.Err)
		}
		got += ev.DeltaText
	}
	if got != "Hello World" {
		t.Errorf("got %q, want %q", got, "Hello World")
	}
}

// TestStreamToolCall verifies fragmented tool_call argument merging.
func TestStreamToolCall(t *testing.T) {
	srv := mockserver.New(mockserver.Turn{
		ToolCall: &mockserver.ToolCall{
			ID:        "call_1",
			Name:      "read",
			Arguments: `{"path":"main.go"}`,
		},
	})
	defer srv.Close()

	c := NewClient("mock", srv.URL(), "")
	ch, err := c.ChatStream(context.Background(), ChatRequest{
		Model:    "mock-small",
		Messages: []Message{{Role: "user", Content: "read it"}},
		Tools: []ToolDef{{
			Type: "function",
			Function: ToolFunction{Name: "read", Description: "d",
				Parameters: []byte(`{"type":"object"}`)},
		}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var got *ToolCall
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream err: %v", ev.Err)
		}
		if d := ev.ToolCallDelta; d != nil {
			// Reconstruct the way the agent does: index-keyed merge.
			if got == nil {
				got = &ToolCall{ID: d.ID, Type: "function"}
			}
			if d.ID != "" {
				got.ID = d.ID
			}
			if d.Name != "" {
				got.Function.Name = d.Name
			}
			got.Function.Arguments += d.Arguments
		}
	}
	if got == nil {
		t.Fatalf("no tool call received")
	}
	if got.Function.Name != "read" {
		t.Errorf("name = %q", got.Function.Name)
	}
	if got.Function.Arguments != `{"path":"main.go"}` {
		t.Errorf("args = %q (fragment merge broken)", got.Function.Arguments)
	}
	if got.ID != "call_1" {
		t.Errorf("id = %q", got.ID)
	}
}

// Reasoning deltas (reasoning_content) must surface as DeltaReasoning so the
// TUI can show a collapsible thinking block.
func TestParseChunkReasoning(t *testing.T) {
	ev, err := parseChunk(`{"choices":[{"delta":{"reasoning_content":"let me think"}}]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.DeltaReasoning != "let me think" {
		t.Errorf("DeltaReasoning = %q", ev.DeltaReasoning)
	}
	if ev.DeltaText != "" {
		t.Errorf("DeltaText should be empty, got %q", ev.DeltaText)
	}

	ev, err = parseChunk(`{"choices":[{"delta":{"content":"answer"}}]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.DeltaText != "answer" || ev.DeltaReasoning != "" {
		t.Errorf("content delta mis-parsed: %+v", ev)
	}
}
