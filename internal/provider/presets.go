package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Preset is a preconfigured OpenAI-compatible provider. Only the API key is
// asked from the user; the base URL is stored locally. There is still NO
// built-in model catalog — models always come from GET {baseURL}/models.
type Preset struct {
	Name         string // config key, e.g. "nvidia"
	Label        string // display name
	BaseURL      string
	NeedsKey     bool   // false for local servers (Ollama, LM Studio)
	Placeholder  string // example key format shown in the prompt
}

// Presets is the built-in provider menu (endpoints only — never models).
func Presets() []Preset {
	return []Preset{
		{Name: "nvidia", Label: "NVIDIA (build.nvidia.com)", BaseURL: "https://integrate.api.nvidia.com/v1", NeedsKey: true, Placeholder: "nvapi-..."},
		{Name: "openai", Label: "OpenAI", BaseURL: "https://api.openai.com/v1", NeedsKey: true, Placeholder: "sk-..."},
		{Name: "deepseek", Label: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", NeedsKey: true, Placeholder: "sk-..."},
		{Name: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", NeedsKey: true, Placeholder: "sk-or-..."},
		{Name: "groq", Label: "Groq", BaseURL: "https://api.groq.com/openai/v1", NeedsKey: true, Placeholder: "gsk_..."},
		{Name: "mistral", Label: "Mistral", BaseURL: "https://api.mistral.ai/v1", NeedsKey: true, Placeholder: "..."},
		{Name: "together", Label: "Together AI", BaseURL: "https://api.together.xyz/v1", NeedsKey: true, Placeholder: "..."},
		{Name: "github", Label: "GitHub Models", BaseURL: "https://models.inference.ai.azure.com", NeedsKey: true, Placeholder: "ghp_..."},
		{Name: "ollama", Label: "Ollama (local)", BaseURL: "http://127.0.0.1:11434/v1", NeedsKey: false, Placeholder: ""},
		{Name: "lmstudio", Label: "LM Studio (local)", BaseURL: "http://127.0.0.1:1234/v1", NeedsKey: false, Placeholder: ""},
		{Name: "custom", Label: "Custom — base URL + key", BaseURL: "", NeedsKey: true, Placeholder: "any key"},
	}
}

// FindPreset returns a preset by name.
func FindPreset(name string) (Preset, bool) {
	for _, p := range Presets() {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// TestResult is the outcome of a credential test.
type TestResult struct {
	Models       []Model // fetched from GET /models (dynamic)
	WorkingModel string  // best chat-verified model ("" if unknown)
	ToolModels   int     // how many models passed with tool support
}

// Test validates credentials before anything is saved:
//  1. GET /models must return HTTP 200 with a non-empty list.
//  2. Every listed model is pinged (bounded workers) with a TOOL prompt, so we
//     can tell tool-capable models apart — the agent needs tool calls to work.
//     Models that reject the tools param are re-pinged without it to confirm
//     plain chat. Endpoints like NVIDIA list models the account can't call
//     (83 listed, ~15 answer), so "at least one real chat answer" is the bar.
//
// Selection prefers a tool-capable model as the default. Any failure → error,
// and the caller must NOT persist the credentials.
func (c *Client) Test(ctx context.Context) (TestResult, error) {
	var res TestResult
	models, err := c.ListModels(ctx)
	if err != nil {
		return res, fmt.Errorf("endpoint check failed: %w", err)
	}
	res.Models = models
	if len(models) == 0 {
		return res, fmt.Errorf("endpoint returned no models")
	}

	// Local servers (no key) only need /models — chat may need a model pulled.
	if c.APIKey == "" {
		res.WorkingModel = models[0].ID
		return res, nil
	}

	// Ping all models in parallel (bounded). A tool-capable answer wins
	// immediately; plain-chat answers are kept as fallback. The whole test is
	// capped so a huge /models list can't stall setup.
	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	const workers = 12
	jobs := make(chan string)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		toolWin string // model that answered WITH a tool call
		chatWin string // model that answered plain chat
		lastErr error
		fails   int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if tctx.Err() != nil { // tool winner found → cancel() already fired
					return
				}
				toolsOK, err := c.pingChat(tctx, id, true)
				if err != nil && !toolsOK {
					// Endpoint may reject the tools param — confirm plain chat.
					if _, err2 := c.pingChat(tctx, id, false); err2 == nil {
						mu.Lock()
						if chatWin == "" {
							chatWin = id
						}
						mu.Unlock()
						continue
					}
				}
				mu.Lock()
				if err == nil {
					if toolsOK {
						if toolWin == "" {
							toolWin = id
							cancel() // stop the remaining pings
						}
					} else if chatWin == "" {
						chatWin = id
					}
				} else {
					fails++
					lastErr = err
				}
				mu.Unlock()
			}
		}()
	}
	for _, m := range models {
		select {
		case jobs <- m.ID:
		case <-tctx.Done():
		}
	}
	close(jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if toolWin != "" {
		res.WorkingModel = toolWin
		return res, nil
	}
	if chatWin != "" {
		res.WorkingModel = chatWin
		return res, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no model answered")
	}
	return res, fmt.Errorf("chat test failed on %d model(s): %w", fails, lastErr)
}

// minimal tool used by the credential ping (mirrors a real agent tool call)
func pingTools() []map[string]any {
	return []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name":        "read_file",
			"description": "Read a file",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []string{"path"},
			},
		},
	}}
}

// pingChat performs one minimal completion against a model.
// withTools=true asks the model to call read_file — success means the endpoint
// supports tool calls (required by the agent loop). Returns (toolsWorked, err);
// err == nil always means "this model can chat".
func (c *Client) pingChat(ctx context.Context, model string, withTools bool) (bool, error) {
	content := "Read main.go using the read_file tool."
	if !withTools {
		content = "hi"
	}
	payload := map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": content}},
		"max_tokens": 60,
	}
	if withTools {
		payload["tools"] = pingTools()
	}
	body, _ := json.Marshal(payload)
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, fmt.Errorf("%s: %w", model, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		s := string(b)
		if len(s) > 200 {
			s = s[:200]
		}
		return false, fmt.Errorf("%s: HTTP %d: %s", model, resp.StatusCode, s)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return false, fmt.Errorf("%s: bad response: %w", model, err)
	}
	if len(out.Choices) == 0 {
		return false, fmt.Errorf("%s: empty choices", model)
	}
	toolsWorked := withTools && len(out.Choices[0].Message.ToolCalls) > 0
	return toolsWorked, nil
}
