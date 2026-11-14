package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mr-daaku/opentitam/internal/agent"
	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/session"
)

// runOnce executes one prompt headlessly (for scripts and tests).
//
// Usage:
//
//	opentitam run "prompt"
//	opentitam run --auto "prompt"     # auto-approve permission asks
func runOnce(args []string) int {
	autoApprove := false
	var promptParts []string
	for _, a := range args {
		if a == "--auto" || a == "-a" {
			autoApprove = true
			continue
		}
		promptParts = append(promptParts, a)
	}
	promptText := strings.TrimSpace(strings.Join(promptParts, " "))
	if promptText == "" {
		fmt.Fprintln(os.Stderr, "usage: opentitam run [--auto] \"<prompt>\"")
		return 2
	}

	cfg, err := config.Load(cwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	client, model, err := resolveProvider(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Allow explicit model override: --model provider/id could be added later;
	// for now use resolved model.

	reg := buildTools(cfg)
	perms := buildPerms(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ag := agent.New(client, model, reg, perms, cwd())
	ag.System = agent.SystemPrompt(cwd()) + "\n\n" + agent.LoadInstructions(cwd())

	if autoApprove {
		ag.SetApprover(func(action, resource, command string) string { return "allow" })
	} else {
		ag.SetApprover(func(action, resource, command string) string {
			// Non-interactive: default deny with a clear message.
			fmt.Fprintf(os.Stderr, "\n[permission] %s on %s — denied in non-interactive mode (use --auto)\n", action, resource)
			return "reject"
		})
	}

	var history []provider.Message
	// Optional: resume last session via env (used by tests).
	if sid := os.Getenv("OPENTITAM_SESSION"); sid != "" {
		store := session.NewStore()
		if s, err := store.Load(sid); err == nil {
			history = s.Messages
		}
	}

	fmt.Println("─ opentitam run ─")

	var finalMsgs []provider.Message
	sawErr := false
	for ev := range ag.Run(ctx, history, promptText) {
		switch ev.Kind {
		case "text_delta":
			fmt.Print(ev.Text)
		case "text_done":
			fmt.Println()
		case "tool_start":
			fmt.Printf("\n⚙ %s %s\n", ev.ToolName, compactArgs(ev.ToolArgs))
		case "tool_end":
			status := "ok"
			if ev.Err != nil {
				status = "err"
			}
			fmt.Printf("  → [%s] %s\n", status, firstLine(ev.Result))
		case "error":
			fmt.Fprintln(os.Stderr, "\nerror:", ev.Err)
			sawErr = true
		case "done":
			if ev.Result != "" {
				_ = json.Unmarshal([]byte(ev.Result), &finalMsgs)
			}
		}
	}

	// Persist if resuming/creating sessions on request.
	if sid := os.Getenv("OPENTITAM_SESSION"); sid != "" && len(finalMsgs) > 0 {
		store := session.NewStore()
		s, err := store.Load(sid)
		if err != nil {
			s = store.New(model, client.Name)
			s.ID = sid
		}
		s.Messages = finalMsgs
		s.Model = model
		s.Provider = client.Name
		s.SetTitleFromFirstUser()
		_ = store.Save(s)
	}
	if sawErr {
		return 1
	}
	return 0
}

func compactArgs(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return string(raw)
	}
	// Prefer the most informative field.
	for _, k := range []string{"path", "pattern", "command"} {
		if v, ok := m[k].(string); ok {
			return v
		}
	}
	b, _ := json.Marshal(m)
	s := string(b)
	if len(s) > 100 {
		s = s[:100] + "…"
	}
	return s
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
