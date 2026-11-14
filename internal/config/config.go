// Package config loads and merges OpenTitam configuration.
//
// Locations (lowest → highest precedence):
//  1. ~/.config/opentitam/opentitam.json(c)     (global)
//  2. <ancestor>/opentitam.json(c)              (direct, farthest → closest)
//  3. <ancestor>/.opencode/opentitam.json(c)    (dot-folder, farthest → closest)
//
// Variables: {env:NAME} and {file:path} substitution inside JSON strings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Provider is a user-defined OpenAI-compatible endpoint.
// There is NO built-in model catalog: Models is either fetched from
// GET {BaseURL}/models or set manually.
type Provider struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey,omitempty"`
	// Models is an optional manual override. Empty means "fetch dynamically".
	Models []string `json:"models,omitempty"`
}

// Permission mirrors one OpenTitam permission rule.
type Permission struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   string `json:"effect"` // allow | ask | deny
}

// Config is the merged result of all configuration files.
type Config struct {
	Schema string               `json:"$schema,omitempty"`
	Model  string               `json:"model,omitempty"`
	Shell  string               `json:"shell,omitempty"`
	// Providers maps provider name → endpoint config (baseURL + apiKey only).
	Providers map[string]Provider `json:"providers,omitempty"`
	// Permissions are ordered rules; last match wins.
	Permissions []Permission `json:"permissions,omitempty"`
	// Instructions lists extra instruction files/globs (like AGENTS.md).
	Instructions []string `json:"instructions,omitempty"`
}

// Dir returns the global config directory (~/.config/opentitam).
func Dir() string {
	if d := os.Getenv("OPENTITAM_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "opentitam")
}

// DataDir returns the local data directory (sessions, saved approvals).
// Windows: %LOCALAPPDATA%\opentitam ; else ~/.local/share/opentitam
func DataDir() string {
	if d := os.Getenv("OPENTITAM_DATA_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LocalAppData"); base != "" {
			return filepath.Join(base, "opentitam")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opentitam")
}

var envRe = regexp.MustCompile(`\{env:([^}]+)\}`)
var fileRe = regexp.MustCompile(`\{file:([^}]+)\}`)

// substitute expands {env:NAME} and {file:path} references in a JSON string.
func substitute(s string, baseDir string) string {
	s = envRe.ReplaceAllStringFunc(s, func(m string) string {
		name := envRe.FindStringSubmatch(m)[1]
		return os.Getenv(name)
	})
	s = fileRe.ReplaceAllStringFunc(s, func(m string) string {
		p := fileRe.FindStringSubmatch(m)[1]
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		} else if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		return strings.TrimRight(string(b), "\r\n")
	})
	return s
}

// parseFile reads one config file, applies substitution, and unmarshals into dst.
func parseFile(path string, dst *Config) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Strip // comments and trailing commas for JSONC support (string-aware).
	cleaned := stripJSONC(string(raw))
	expanded := substitute(cleaned, filepath.Dir(path))
	return json.Unmarshal([]byte(expanded), dst)
}

// stripJSONC removes // line comments and trailing commas outside strings.
func stripJSONC(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inStr, esc, lineComment := false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
				b.WriteByte(c)
			}
			continue
		}
		if inStr {
			b.WriteByte(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			b.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			lineComment = true
			i++
			continue
		}
		b.WriteByte(c)
	}
	// Remove trailing commas before } or ]
	out := b.String()
	var res strings.Builder
	inStr, esc = false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inStr {
			res.WriteByte(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			res.WriteByte(c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				continue // drop trailing comma
			}
		}
		res.WriteByte(c)
	}
	return res.String()
}

// candidatePaths returns config file paths for dir (both naming styles).
func candidatePaths(dir string) []string {
	return []string{
		filepath.Join(dir, "opentitam.jsonc"),
		filepath.Join(dir, "opentitam.json"),
		filepath.Join(dir, ".opencode", "opentitam.jsonc"),
		filepath.Join(dir, ".opencode", "opentitam.json"),
	}
}

// merge applies src over dst (src wins for non-zero scalar fields; maps and
// slices are appended/overridden per field semantics).
func merge(dst *Config, src *Config) {
	if src.Schema != "" {
		dst.Schema = src.Schema
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Shell != "" {
		dst.Shell = src.Shell
	}
	if len(src.Providers) > 0 {
		if dst.Providers == nil {
			dst.Providers = map[string]Provider{}
		}
		for k, v := range src.Providers {
			dst.Providers[k] = v
		}
	}
	if len(src.Permissions) > 0 {
		dst.Permissions = append(dst.Permissions, src.Permissions...)
	}
	if len(src.Instructions) > 0 {
		dst.Instructions = append(dst.Instructions, src.Instructions...)
	}
}

// Load discovers and merges config from the global dir and every ancestor of
// startDir (direct files first, then .opencode files; farthest ancestor first).
func Load(startDir string) (*Config, error) {
	cfg := &Config{}

	// Global (lowest precedence).
	if d := Dir(); d != "" {
		for _, p := range candidatePaths(d) {
			if fileExists(p) {
				var c Config
				if err := parseFile(p, &c); err != nil {
					return nil, fmt.Errorf("config %s: %w", p, err)
				}
				merge(cfg, &c)
				break // only the first global file (direct style preferred)
			}
		}
		// Global dir may itself be checked for .opencode style too; keep simple:
		// first hit in candidatePaths order wins.
	}

	// Project: collect ancestors farthest → closest.
	if startDir == "" {
		startDir, _ = os.Getwd()
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return cfg, nil
	}
	var dirs []string
	for {
		dirs = append([]string{abs}, dirs...)
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}

	// Pass 1: direct opentitam.json(c), farthest → closest.
	for _, d := range dirs {
		for _, p := range []string{filepath.Join(d, "opentitam.jsonc"), filepath.Join(d, "opentitam.json")} {
			if fileExists(p) {
				var c Config
				if err := parseFile(p, &c); err != nil {
					return nil, fmt.Errorf("config %s: %w", p, err)
				}
				merge(cfg, &c)
			}
		}
	}
	// Pass 2: .opencode/opentitam.json(c), farthest → closest (overrides direct).
	for _, d := range dirs {
		for _, p := range []string{
			filepath.Join(d, ".opencode", "opentitam.jsonc"),
			filepath.Join(d, ".opencode", "opentitam.json"),
		} {
			if fileExists(p) {
				var c Config
				if err := parseFile(p, &c); err != nil {
					return nil, fmt.Errorf("config %s: %w", p, err)
				}
				merge(cfg, &c)
			}
		}
	}
	return cfg, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// SaveGlobal writes cfg as the global config file (used by `opentitam setup`).
func SaveGlobal(cfg *Config) error {
	d := Dir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, "opentitam.json"), append(b, '\n'), 0o600)
}

// DefaultModel returns the configured model or "".
func (c *Config) DefaultModel() string { return c.Model }
