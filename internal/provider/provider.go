// Package provider implements the OpenTitam model provider layer.
//
// There is NO built-in model catalog. A provider is only {baseURL, apiKey};
// the model list is fetched dynamically from GET {baseURL}/models (the
// OpenAI-compatible endpoint). Chat uses POST {baseURL}/chat/completions
// with SSE streaming and tool calling.
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to one OpenAI-compatible endpoint.
type Client struct {
	Name    string // provider name from config (e.g. "openrouter")
	BaseURL string // e.g. https://openrouter.ai/api/v1
	APIKey  string
	HTTP    *http.Client
}

func NewClient(name, baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		Name:    name,
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 0}, // streaming must not time out
	}
}

// Model is one dynamically loaded model.
type Model struct {
	ID string `json:"id"`
}

// ListModels fetches GET {baseURL}/models. No hardcoded catalog anywhere.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", c.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("provider %s: GET /models -> %d: %s", c.Name, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("provider %s: decode models: %w", c.Name, err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("provider %s: /models returned no models", c.Name)
	}
	return payload.Data, nil
}

// ---- Chat messages (OpenAI format) ----

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDef struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatRequest is the POST /chat/completions body.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ---- Streaming ----

// StreamEvent is one SSE event from the chat endpoint.
type StreamEvent struct {
	// DeltaText is incremental assistant text.
	DeltaText string
	// ToolCallDelta carries incremental tool call info (index-based merge).
	ToolCallDelta *ToolCallDelta
	// Done is true on [DONE].
	Done bool
	// Err is a stream-level error.
	Err error
}

type ToolCallDelta struct {
	Index    int
	ID       string
	Name     string
	Arguments string
}

// ChatStream starts a streaming chat completion and returns an event channel.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", c.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("provider %s: /chat/completions -> %d: %s", c.Name, resp.StatusCode, strings.TrimSpace(string(b)))
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				ch <- StreamEvent{Done: true}
				return
			}
			ev, err := parseChunk(data)
			if err != nil {
				continue // ignore malformed keep-alives
			}
			ch <- ev
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			ch <- StreamEvent{Err: fmt.Errorf("stream read: %w", err)}
		}
	}()
	return ch, nil
}

func parseChunk(data string) (StreamEvent, error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return StreamEvent{}, err
	}
	var ev StreamEvent
	if len(chunk.Choices) == 0 {
		return ev, nil
	}
	d := chunk.Choices[0].Delta
	if d.Content != "" {
		ev.DeltaText = d.Content
	}
	for _, tc := range d.ToolCalls {
		ev.ToolCallDelta = &ToolCallDelta{
			Index:     tc.Index,
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
	}
	return ev, nil
}

// ChatNonStream performs a single (non-streaming) completion — used for
// titles/summaries and by tests.
func (c *Client) ChatNonStream(ctx context.Context, req ChatRequest) (Message, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return Message{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := *c.HTTP
	client.Timeout = 120 * time.Second
	resp, err := client.Do(httpReq)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Message{}, fmt.Errorf("provider %s: %d: %s", c.Name, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Message{}, err
	}
	if len(out.Choices) == 0 {
		return Message{}, fmt.Errorf("provider %s: empty choices", c.Name)
	}
	return out.Choices[0].Message, nil
}
