package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	in := `{
  // comment
  "model": "a/b", // trailing comment
  "arr": [1, 2,], 
}`
	got := stripJSONC(in)
	var c Config
	if err := json.Unmarshal([]byte(got), &c); err != nil {
		t.Fatalf("stripJSONC produced invalid JSON: %v\n%s", err, got)
	}
	if c.Model != "a/b" {
		t.Errorf("model = %q", c.Model)
	}
}

func TestSubstituteEnvAndFile(t *testing.T) {
	t.Setenv("OT_TEST_KEY", "secret123")
	dir := t.TempDir()
	fp := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(fp, []byte("filekey\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := substitute(`{"k":"{env:OT_TEST_KEY}","f":"{file:key.txt}"}`, dir)
	if want := `{"k":"secret123","f":"filekey"}`; got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestLoadMergePrecedence(t *testing.T) {
	// Global config via env override dir.
	globalDir := t.TempDir()
	t.Setenv("OPENTITAM_CONFIG_DIR", globalDir)
	if err := os.WriteFile(filepath.Join(globalDir, "opentitam.json"),
		[]byte(`{"model":"global/model","providers":{"p":{"baseURL":"http://g"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// Project with direct + .opencode style configs.
	proj := t.TempDir()
	sub := filepath.Join(proj, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Direct config at project root (farther).
	if err := os.WriteFile(filepath.Join(proj, "opentitam.json"),
		[]byte(`{"model":"proj/model","providers":{"p":{"baseURL":"http://proj"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// .opencode config closer — must override direct.
	if err := os.MkdirAll(filepath.Join(sub, ".opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".opencode", "opentitam.json"),
		[]byte(`{"model":"dot/model"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	// .opencode (closest pass) overrides direct (first pass) overrides global.
	if cfg.Model != "dot/model" {
		t.Errorf("model = %q, want dot/model", cfg.Model)
	}
	// Providers merged: global p replaced by proj p (same key, later wins).
	if cfg.Providers["p"].BaseURL != "http://proj" {
		t.Errorf("provider baseURL = %q, want http://proj", cfg.Providers["p"].BaseURL)
	}
}
