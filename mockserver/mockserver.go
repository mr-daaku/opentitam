// Package mockserver is a fake OpenAI-compatible server used by tests and
// demos. It serves GET /models and POST /chat/completions with SSE streaming
// and tool calling. The PC is the backend — no external service needed.
package mockserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Server is the fake endpoint.
type Server struct {
	HTTP *httptest.Server

	mu       sync.Mutex
	requests []ChatRequest
	// Script controls the conversation: each call to /chat/completions
	// consumes the next turn. If empty, replies with a plain text echo.
	Script []Turn
	calls  int
}

// Turn is one scripted model response.
type Turn struct {
	// Text is assistant text (streamed in chunks).
	Text string
	// ToolCall, when set, makes the model request a tool.
	ToolCall *ToolCall
	// ToolResultID pairs with the previous tool call when replying.
	ToolResultID   string
	ToolResultName string
	ToolResult     string
}

type ToolCall struct {
	ID       string
	Name     string
	Arguments string
}

// ChatRequest records one /chat/completions body (decoded loosely).
type ChatRequest struct {
	Model    string
	Messages []map[string]any
	Stream   bool
}

// New starts a mock server with the given scripted turns.
func New(script ...Turn) *Server {
	s := &Server{Script: script}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /models", s.handleModels)
	mux.HandleFunc("POST /chat/completions", s.handleChat)
	s.HTTP = httptest.NewServer(mux)
	return s
}

// URL returns the base URL (use as provider baseURL).
func (s *Server) URL() string { return s.HTTP.URL }

// Close shuts the server down.
func (s *Server) Close() { s.HTTP.Close() }

// Requests returns captured chat requests.
func (s *Server) Requests() []ChatRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ChatRequest{}, s.requests...)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	// Standard OpenAI /models shape — models are DYNAMIC, never hardcoded.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": "mock-small", "object": "model", "created": 1, "owned_by": "mock"},
			{"id": "mock-large", "object": "model", "created": 1, "owned_by": "mock"},
		},
	})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string          `json:"model"`
		Messages json.RawMessage `json:"messages"`
		Stream   bool            `json:"stream"`
		Tools    []any           `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var msgs []map[string]any
	_ = json.Unmarshal(req.Messages, &msgs)

	s.mu.Lock()
	s.calls++
	call := s.calls
	s.requests = append(s.requests, ChatRequest{Model: req.Model, Messages: msgs, Stream: req.Stream})
	var turn Turn
	if len(s.Script) > 0 {
		idx := call - 1
		if idx >= len(s.Script) {
			idx = len(s.Script) - 1
		}
		turn = s.Script[idx]
	} else {
		turn = Turn{Text: "mock reply"}
	}
	s.mu.Unlock()

	if !req.Stream {
		// Non-streaming JSON response.
		msg := map[string]any{"role": "assistant", "content": turn.Text}
		if turn.ToolCall != nil {
			msg["content"] = nil
			msg["tool_calls"] = []map[string]any{{
				"id":   turn.ToolCall.ID,
				"type": "function",
				"function": map[string]any{
					"name":      turn.ToolCall.Name,
					"arguments": turn.ToolCall.Arguments,
				},
			}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": msg, "finish_reason": "stop"}},
		})
		return
	}

	// SSE stream.
	w.Header().Set("Content-Type", "text/event-stream")
	fl, _ := w.(http.Flusher)

	write := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if fl != nil {
			fl.Flush()
		}
	}

	// Stream text in two chunks to exercise delta merging.
	if turn.ToolCall == nil && turn.Text != "" {
		runes := []rune(turn.Text)
		mid := len(runes) / 2
		if mid == 0 {
			mid = len(runes)
		}
		for _, chunk := range []string{string(runes[:mid]), string(runes[mid:])} {
			write(map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{"content": chunk}}},
			})
		}
	}
	// Tool call streamed in fragments (id/name then arguments).
	if turn.ToolCall != nil {
		write(map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{
				"tool_calls": []map[string]any{{
					"index":    0,
					"id":       turn.ToolCall.ID,
					"function": map[string]any{"name": turn.ToolCall.Name, "arguments": ""},
				}},
			}}},
		})
		args := turn.ToolCall.Arguments
		for len(args) > 0 {
			n := 12
			if len(args) < n {
				n = len(args)
			}
			write(map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{
					"tool_calls": []map[string]any{{
						"index":    0,
						"function": map[string]any{"arguments": args[:n]},
					}},
				}}},
			})
			args = args[n:]
		}
	}
	// Tool result reply turn.
	if turn.ToolResultID != "" {
		write(map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{
				"content": turn.Text,
			}}},
		})
	}
	write(map[string]any{"choices": []map[string]any{{"delta": map[string]any{}, "finish_reason": "stop"}}})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
}

// Count returns how many chat completions were served.
func (s *Server) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// ToolMessages builds a helper: user msg → assistant tool_call → tool result.
func ToolMessages(toolID, name, result string) []map[string]any {
	return []map[string]any{
		{"role": "user", "content": "do it"},
		{
			"role":      "assistant",
			"content":   nil,
			"tool_calls": []map[string]any{{"id": toolID, "type": "function",
				"function": map[string]any{"name": name, "arguments": "{}"}}},
		},
		{"role": "tool", "tool_call_id": toolID, "content": result},
	}
}

// HasToolResult reports whether any message is a tool result.
func HasToolResult(msgs []map[string]any) bool {
	for _, m := range msgs {
		if m["role"] == "tool" {
			return true
		}
	}
	return false
}

// Join is a small helper for tests.
func Join(parts ...string) string { return strings.Join(parts, "") }
