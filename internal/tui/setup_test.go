package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mr-daaku/opentitam/internal/config"
)

func enterKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

// First run: TUI must open directly (no error) with the provider picker up.
func TestFirstRunOpensDirectlyWithProviderPicker(t *testing.T) {
	cfg := &config.Config{}
	m, err := New(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("New() with no providers must not error, got: %v", err)
	}
	if m.overlay != overlayProvider {
		t.Fatalf("expected overlayProvider on first run, got %v", m.overlay)
	}
	if m.agent != nil || m.client != nil {
		t.Fatal("no provider yet → agent/client must be nil")
	}
	if got := len(m.items); got != len(providerLabels()) {
		t.Fatalf("provider list empty: %d items", got)
	}
}

// Picking a key-needing preset must switch to key entry with its base URL.
func TestProviderPickOpensKeyEntry(t *testing.T) {
	m, err := New(&config.Config{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// select NVIDIA (first preset)
	m.sel = 0
	m.selectItem()
	if m.overlay != overlayKey || m.setupMode != "key" {
		t.Fatalf("expected key entry, got overlay=%v mode=%q", m.overlay, m.setupMode)
	}
	if m.setupBase == "" {
		t.Fatal("preset base URL must be pre-filled (saved locally)")
	}
	if m.setupPreset.Name != "nvidia" {
		t.Fatalf("preset = %q", m.setupPreset.Name)
	}
}

// Custom preset must ask for the base URL first.
func TestCustomPresetAsksBaseURLFirst(t *testing.T) {
	m, err := New(&config.Config{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range providerLabels() {
		_ = p
		if strings.Contains(p, "Custom") {
			m.sel = i
			break
		}
	}
	m.selectItem()
	if m.overlay != overlayKey || m.setupMode != "base" {
		t.Fatalf("expected base URL entry, got overlay=%v mode=%q", m.overlay, m.setupMode)
	}
}

// Empty key must not trigger a test.
func TestEmptyKeyRejected(t *testing.T) {
	m, err := New(&config.Config{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.sel = 0 // nvidia (needs key)
	m.selectItem()
	m.keyInput.SetValue("   ")
	model, _ := m.handleSetupKey(enterKey())
	if model.(*Model).testing {
		t.Fatal("empty key must not start a test")
	}
	if m.setupErr == "" {
		t.Fatal("empty key must show an error")
	}
}
