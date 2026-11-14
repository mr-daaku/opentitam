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
	"time"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/permissions"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/tools"
	"github.com/mr-daaku/opentitam/internal/tui"
)

// Version is defined once in the tui package (sidebar shows it too).
const Version = tui.Version

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
	case "api":
		return runAPI(args[1:])
	case "setup":
		// `setup` is kept as an alias of `api set`.
		return runAPI(append([]string{"set"}, args[1:]...))
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
  opentitam api                Show current provider + model
  opentitam api set [name]     Add provider: paste key → tested → saved only if OK
  opentitam api test           Re-test the saved provider
  opentitam models             Fetch models (saves list) and choose the default
  opentitam version            Print version

PRESETS (api set)
  nvidia openai deepseek openrouter groq mistral together github ollama lmstudio custom

CONFIG
  Global : ~/.config/opentitam/opentitam.json
  Project: <repo>/opentitam.json or <repo>/.opencode/opentitam.json

  Providers have only baseURL + apiKey — there is NO built-in model catalog.
  Models are loaded from GET {baseURL}/models and saved into config.
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

// runModels fetches models from every provider (dynamic), SAVES the lists into
// config, and interactively lets the user choose the default model.
func runModels(args []string) int {
	cfg, err := config.Load(cwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if len(cfg.Providers) == 0 {
		fmt.Fprintln(os.Stderr, "No providers configured. Run `opentitam api set` first.")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	interactive := isInteractive()
	changed := false
	exit := 0
	names := sortedKeys(cfg.Providers)
	all := map[string][]string{} // provider → model ids (for the picker)
	for _, name := range names {
		p := cfg.Providers[name]
		c := provider.NewClient(name, p.BaseURL, p.APIKey)
		models, err := c.ListModels(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", name, err)
			exit = 1
			continue
		}
		ids := modelIDs(models)
		all[name] = ids
		// Persist the freshly fetched list ("jo hoga wo save ho jayega").
		p.Models = ids
		cfg.Providers[name] = p
		changed = true

		fmt.Printf("[%s] %s — %d model(s)\n", name, p.BaseURL, len(models))
		for _, id := range ids {
			mark := "  "
			if cfg.Model == name+"/"+id {
				mark = "* "
			}
			fmt.Printf("%s%s/%s\n", mark, name, id)
		}
	}
	if changed {
		if err := config.SaveGlobal(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "cannot save config:", err)
			return 1
		}
		fmt.Println("\n(model lists saved to config)")
	}

	// Choose which model to USE — saved too ("jo use karna hai wo bhi save").
	if interactive && exit == 0 {
		fmt.Printf("\nSet default model [enter keeps %q]: ", cfg.Model)
		line, _ := readLine()
		line = strings.TrimSpace(line)
		if line != "" {
			// Accept "provider/model", bare id (single provider), or "provider model".
			line = strings.ReplaceAll(line, " ", "/")
			chosen := ""
			if strings.Contains(line, "/") {
				if pname, rest, ok := strings.Cut(line, "/"); ok {
					ids, ok := all[pname]
					if !ok {
						fmt.Fprintf(os.Stderr, "provider %q not configured\n", pname)
						return 1
					}
					for _, id := range ids {
						if id == rest {
							chosen = pname + "/" + rest
						}
					}
					if chosen == "" {
						fmt.Fprintf(os.Stderr, "model %q not found on provider %q\n", rest, pname)
						return 1
					}
				}
			} else {
				// Bare id: find it across providers.
				for pname, ids := range all {
					for _, id := range ids {
						if id == line {
							chosen = pname + "/" + line
							break
						}
					}
					if chosen != "" {
						break
					}
				}
				if chosen == "" {
					fmt.Fprintf(os.Stderr, "model %q not found on any provider\n", line)
					return 1
				}
			}
			cfg.Model = chosen
			if err := config.SaveGlobal(cfg); err != nil {
				fmt.Fprintln(os.Stderr, "cannot save config:", err)
				return 1
			}
			fmt.Printf("Default model saved: %s\n", chosen)
		}
	}
	return exit
}

// readLine reads one trimmed line from stdin; on empty input returns "".
func readLine() (string, error) {
	var line string
	if _, err := fmt.Fscanln(os.Stdin, &line); err != nil {
		return "", nil // empty line (just enter)
	}
	return strings.TrimSpace(line), nil
}

// isInteractive reports whether stdin is a terminal.
func isInteractive() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
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
