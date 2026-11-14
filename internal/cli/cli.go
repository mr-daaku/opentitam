// Package cli implements the OpenTitam command-line interface:
//
//	opentitam               launch the TUI
//	opentitam run "prompt"  headless one-shot
//	opentitam models        list models loaded from providers (dynamic)
//	opentitam setup         wizard: baseURL + apiKey → fetch models
//	opentitam version       print version
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/permissions"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/tools"
)

const Version = "0.1.0"

// Execute is the CLI entry point.
func Execute(args []string) int {
	if len(args) == 0 {
		return runTUI(args)
	}
	switch args[0] {
	case "run":
		return runOnce(args[1:])
	case "models":
		return runModels(args[1:])
	case "setup":
		return runSetup(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("opentitam %s\n", Version)
		return 0
	case "help", "--help", "-h":
		printHelp()
		return 0
	default:
		// Treat unknown first arg as a directory for the TUI (opencode-style).
		return runTUI(args)
	}
}

func printHelp() {
	fmt.Print(`OpenTitam — terminal AI coding assistant (OpenCode-style, local-first)

USAGE
  opentitam                    Launch the TUI in the current directory
  opentitam <directory>        Launch the TUI in a directory
  opentitam run "<prompt>"     Run one prompt headlessly and print the reply
  opentitam models             List models fetched from your providers
  opentitam setup              Configure provider (base URL + API key) and fetch models
  opentitam version            Print version

CONFIG
  Global : ~/.config/opentitam/opentitam.json
  Project: <repo>/opentitam.json or <repo>/.opencode/opentitam.json

  Providers have only baseURL + apiKey — there is NO built-in model catalog.
  Models are loaded from GET {baseURL}/models.
`)
}

// resolveProvider picks the client + model to use.
func resolveProvider(cfg *config.Config) (*provider.Client, string, error) {
	if len(cfg.Providers) == 0 {
		return nil, "", fmt.Errorf("no providers configured — run `opentitam setup` first")
	}
	// Prefer configured default model's provider.
	if cfg.Model != "" {
		if name, model, ok := strings.Cut(cfg.Model, "/"); ok {
			if p, ok := cfg.Providers[name]; ok {
				return provider.NewClient(name, p.BaseURL, p.APIKey), model, nil
			}
			// model may contain slashes (nvidia/...) — rejoin.
			for pname, p := range cfg.Providers {
				if strings.HasPrefix(cfg.Model, pname+"/") {
					return provider.NewClient(pname, p.BaseURL, p.APIKey), strings.TrimPrefix(cfg.Model, pname+"/"), nil
				}
			}
			_ = model
		}
		// No provider prefix: if exactly one provider, use it.
		if len(cfg.Providers) == 1 {
			for name, p := range cfg.Providers {
				return provider.NewClient(name, p.BaseURL, p.APIKey), cfg.Model, nil
			}
		}
	}
	// Fallback: first provider + error that model is not set.
	for name, p := range cfg.Providers {
		c := provider.NewClient(name, p.BaseURL, p.APIKey)
		return c, "", fmt.Errorf("no default model set — run `opentitam models` to list, then set \"model\": \"%s/<model-id>\"", name)
	}
	return nil, "", fmt.Errorf("no providers configured")
}

// buildTools creates the tool registry from config.
func buildTools(cfg *config.Config) *tools.Registry {
	return tools.DefaultRegistry(cfg.Shell)
}

// buildPerms creates the permission engine from config rules.
func buildPerms(cfg *config.Config) *permissions.Engine {
	return permissions.New(cfg.Permissions)
}

// runModels prints dynamically loaded models from every provider.
func runModels(args []string) int {
	cfg, err := config.Load(cwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if len(cfg.Providers) == 0 {
		fmt.Fprintln(os.Stderr, "No providers configured. Run `opentitam setup` first.")
		return 1
	}
	ctx := context.Background()
	exit := 0
	names := sortedKeys(cfg.Providers)
	for _, name := range names {
		p := cfg.Providers[name]
		c := provider.NewClient(name, p.BaseURL, p.APIKey)
		models, err := c.ListModels(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", name, err)
			exit = 1
			continue
		}
		fmt.Printf("[%s] %s — %d model(s)\n", name, p.BaseURL, len(models))
		if len(p.Models) > 0 {
			fmt.Println("  (manual model list in config overrides endpoint)")
		}
		for _, m := range models {
			mark := "  "
			if cfg.Model == name+"/"+m.ID {
				mark = "* "
			}
			fmt.Printf("%s%s/%s\n", mark, name, m.ID)
		}
	}
	return exit
}

func sortedKeys(m map[string]config.Provider) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

// prompt asks a question on stdin and returns trimmed input.
func prompt(label, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	var line string
	if _, err := fmt.Fscanln(os.Stdin, &line); err != nil {
		// Empty line (just enter) → return default.
		return def, nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// runSetup is the interactive wizard: baseURL + apiKey → fetch models.
func runSetup(args []string) int {
	fmt.Println("OpenTitam setup — add an OpenAI-compatible provider")
	fmt.Println("(there is no built-in model catalog; models load from your endpoint)")
	fmt.Println()

	name, err := prompt("Provider name (e.g. openrouter, deepseek, ollama)", "openai")
	if err != nil || name == "" {
		name = "openai"
	}
	baseURL, err := prompt("Base URL (e.g. https://api.openai.com/v1)", "")
	if err != nil || baseURL == "" {
		fmt.Fprintln(os.Stderr, "Base URL is required.")
		return 1
	}
	apiKey, err := prompt("API key (leave empty for local servers like Ollama)", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		return 1
	}

	cfg, err := config.Load(cwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}
	cfg.Providers[name] = config.Provider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
	}

	// Fetch models immediately to validate.
	c := provider.NewClient(name, cfg.Providers[name].BaseURL, apiKey)
	ctx := context.Background()
	models, err := c.ListModels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nCould not fetch models: %v\n", err)
		fmt.Println("Saving provider anyway — you can fix the URL and retry `opentitam models`.")
	} else {
		fmt.Printf("\nLoaded %d model(s) from %s:\n", len(models), baseURL)
		for i, m := range models {
			if i >= 40 {
				fmt.Printf("  … and %d more\n", len(models)-40)
				break
			}
			fmt.Printf("  %s\n", m.ID)
		}
		// Pick default model.
		def := ""
		if len(models) > 0 {
			def = models[0].ID
			for _, m := range models {
				if strings.Contains(strings.ToLower(m.ID), "mini") ||
					strings.Contains(strings.ToLower(m.ID), "haiku") ||
					strings.Contains(strings.ToLower(m.ID), "flash") {
					def = m.ID
					break
				}
			}
		}
		choice, err := prompt("Set default model", def)
		if err == nil && choice != "" {
			// Allow full "provider/model" or bare id.
			if strings.HasPrefix(choice, name+"/") {
				cfg.Model = choice
			} else {
				cfg.Model = name + "/" + choice
			}
		}
	}

	if err := config.SaveGlobal(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "cannot save config:", err)
		return 1
	}
	fmt.Printf("\nSaved %s\n", filepath.Join(config.Dir(), "opentitam.json"))
	if cfg.Model != "" {
		fmt.Printf("Default model: %s\n", cfg.Model)
	}
	fmt.Println("Next: `opentitam models` to list, `opentitam` to start the TUI.")
	return 0
}

// prettyJSON is a helper for debug output.
func prettyJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
