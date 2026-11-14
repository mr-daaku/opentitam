package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/provider"
)

// runAPI implements `opentitam api [status|set|test] [provider]`.
//
//	api            show current provider + model
//	api set        interactive: pick preset → paste key → test → save
//	api set <name> same, skipping the preset menu (e.g. `api set nvidia`)
//	api test       re-test the saved provider
func runAPI(args []string) int {
	if len(args) == 0 {
		return apiStatus()
	}
	switch args[0] {
	case "set", "add", "login":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		return apiSet(name)
	case "test":
		return apiTest()
	case "status":
		return apiStatus()
	default:
		// `opentitam api nvidia` → treat as set nvidia
		return apiSet(args[0])
	}
}

func apiStatus() int {
	cfg, err := config.Load(cwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if len(cfg.Providers) == 0 {
		fmt.Println("No provider configured yet.")
		fmt.Println("Run `opentitam api set` to add one (only an API key is needed).")
		return 0
	}
	for _, name := range sortedKeys(cfg.Providers) {
		p := cfg.Providers[name]
		mark := "  "
		if strings.HasPrefix(cfg.Model, name+"/") {
			mark = "* "
		}
		key := "(no key)"
		if p.APIKey != "" {
			key = maskKey(p.APIKey)
		}
		fmt.Printf("%s%s\n", mark, name)
		fmt.Printf("    baseURL: %s\n", p.BaseURL)
		fmt.Printf("    apiKey : %s\n", key)
		if len(p.Models) > 0 {
			fmt.Printf("    models : %d saved in config\n", len(p.Models))
		}
	}
	if cfg.Model != "" {
		fmt.Printf("\nCurrent model: %s\n", cfg.Model)
	} else {
		fmt.Println("\nNo model selected — run `opentitam models`.")
	}
	return 0
}

func apiSet(presetName string) int {
	cfg, err := config.Load(cwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}

	// 1. Choose provider preset.
	var preset provider.Preset
	if presetName != "" {
		p, ok := provider.FindPreset(presetName)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown provider %q. Options:\n", presetName)
			for _, pp := range provider.Presets() {
				fmt.Printf("  %-12s %s\n", pp.Name, pp.Label)
			}
			return 1
		}
		preset = p
	} else {
		fmt.Println("Select a provider (base URL is stored locally, only the key is entered):")
		presets := provider.Presets()
		for i, p := range presets {
			fmt.Printf("  %2d) %-12s %s\n", i+1, p.Name, p.Label)
		}
		fmt.Println("   0) custom — own base URL + key")
		fmt.Print("Choice [1]: ")
		line, _ := readLine()
		idx := 1
		if line != "" {
			if n, err := fmt.Sscanf(line, "%d", &idx); err != nil || n != 1 || idx < 0 || idx > len(presets) {
				fmt.Fprintln(os.Stderr, "invalid choice")
				return 1
			}
		}
		if idx == 0 {
			preset = provider.Preset{Name: "custom", Label: "Custom", NeedsKey: true, Placeholder: "any key"}
		} else {
			preset = presets[idx-1]
		}
	}

	// 2. Base URL (custom only; presets carry theirs).
	baseURL := preset.BaseURL
	if preset.Name == "custom" {
		fmt.Print("Base URL (e.g. https://api.openai.com/v1): ")
		line, _ := readLine()
		baseURL = strings.TrimRight(strings.TrimSpace(line), "/")
		if baseURL == "" {
			fmt.Fprintln(os.Stderr, "base URL is required")
			return 1
		}
	}

	// 3. API key (hidden-ish prompt).
	apiKey := ""
	if preset.NeedsKey {
		if preset.Placeholder != "" {
			fmt.Printf("API key (%s): ", preset.Placeholder)
		} else {
			fmt.Print("API key: ")
		}
		line, _ := readLine()
		apiKey = strings.TrimSpace(line)
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "API key is required for this provider")
			return 1
		}
	}

	// 4. TEST — never save on failure.
	name := preset.Name
	if name == "" {
		name = "custom"
	}
	fmt.Printf("\nTesting %s (%s) …\n", name, baseURL)
	c := provider.NewClient(name, baseURL, apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	res, err := c.Test(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nFAILED — nothing was saved.\n  %v\n", err)
		fmt.Println("Fix the key/URL and retry `opentitam api set`.")
		return 1
	}

	// 5. Save (provider + dynamic model list + default model).
	cfg.Providers[name] = config.Provider{BaseURL: baseURL, APIKey: apiKey, Models: modelIDs(res.Models)}
	def := res.WorkingModel
	if def == "" {
		def = res.Models[0].ID
	}
	cfg.Model = name + "/" + def
	if err := config.SaveGlobal(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "cannot save config:", err)
		return 1
	}

	fmt.Printf("OK — %d model(s) loaded, %d saved in config.\n", len(res.Models), len(res.Models))
	if res.WorkingModel != "" {
		fmt.Printf("Chat-verified model: %s\n", res.WorkingModel)
	}
	fmt.Printf("Default model: %s\n", cfg.Model)
	fmt.Printf("Saved: %s\n", configPath())
	fmt.Println("Next: `opentitam models` to browse/switch, `opentitam` to start.")
	return 0
}

func apiTest() int {
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
	fmt.Printf("Testing %s (%s) …\n", client.Name, client.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	res, err := client.Test(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		return 1
	}
	fmt.Printf("OK — %d model(s) available.\n", len(res.Models))
	if res.WorkingModel != "" {
		fmt.Printf("Chat-verified: %s\n", res.WorkingModel)
	}
	if model != "" {
		fmt.Printf("Configured model: %s/%s\n", client.Name, model)
	}
	return 0
}

func modelIDs(ms []provider.Model) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return "****"
	}
	return k[:6] + strings.Repeat("*", 8) + k[len(k)-4:]
}

func configPath() string {
	return config.Dir() + string(os.PathSeparator) + "opentitam.json"
}
