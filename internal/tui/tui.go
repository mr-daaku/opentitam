// Package tui implements the OpenTitam terminal UI.
// Design follows docs/tui-spec.md ( researched from OpenCode's source ):
// dark theme #0a0a0a/#141414, session strip, sidebar, scrollbox transcript,
// composer with ▹ rail, permission stack above composer, dialogs 60/88,
// leader ctrl+x, ctrl+p commands, ctrl+o sessions.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/mr-daaku/opentitam/internal/agent"
	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/session"
)

// ---- theme: exact hexes from the OpenCode "opencode" dark theme ----

var (
	// surfaces
	bgBase    = lipgloss.Color("#0a0a0a")
	bgRaised  = lipgloss.Color("#141414")
	bgHigh    = lipgloss.Color("#1e1e1e")
	// text
	textBase  = lipgloss.Color("#eeeeee")
	textMuted = lipgloss.Color("#808080")
	// chrome
	borderBase    = lipgloss.Color("#484848")
	borderActive  = lipgloss.Color("#606060")
	// feedback
	colError   = lipgloss.Color("#e06c75")
	colWarning = lipgloss.Color("#f5a742")
	colSuccess = lipgloss.Color("#7fd88f")
	colInfo    = lipgloss.Color("#56b6c2")
	// hues
	colAccent     = lipgloss.Color("#9d7cd8") // purple, headings
	colInteractive = lipgloss.Color("#fab283") // orange, focus/markers
)

var (
	styleTopStrip = lipgloss.NewStyle().
			Background(bgRaised).
			Foreground(textBase).
			Bold(true).
			Width(0) // set at render

	styleStripMuted = lipgloss.NewStyle().
			Background(bgRaised).
			Foreground(textMuted)

	styleSidebar = lipgloss.NewStyle().
			Background(bgRaised).
			Foreground(textMuted).
			Width(42)

	styleSideSel = lipgloss.NewStyle().
			Background(bgRaised).
			Foreground(colInteractive)

	styleUserLabel = lipgloss.NewStyle().Bold(true).Foreground(colInteractive)
	styleAsstLabel = lipgloss.NewStyle().Bold(true).Foreground(colSuccess)
	styleToolLabel = lipgloss.NewStyle().Foreground(colInfo)
	styleToolOK    = lipgloss.NewStyle().Foreground(colSuccess)
	styleToolErr   = lipgloss.NewStyle().Foreground(colError)
	styleError     = lipgloss.NewStyle().Foreground(colError)
	styleWarn      = lipgloss.NewStyle().Foreground(colWarning)
	styleMuted     = lipgloss.NewStyle().Foreground(textMuted)
	styleAccent    = lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	styleUserBody = lipgloss.NewStyle().
			Background(bgRaised).
			Foreground(textBase).
			Padding(0, 1)

	styleDialog = lipgloss.NewStyle().
			Background(bgBase).
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderBase).
			Padding(1, 2)

	styleDialogTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)

	styleSelRow = lipgloss.NewStyle().
			Foreground(colInteractive).
			Background(bgHigh)

	styleComposer = lipgloss.NewStyle().
			Background(bgBase)

	styleRail = lipgloss.NewStyle().Foreground(borderActive)

	styleFooterHint = lipgloss.NewStyle().Foreground(textMuted)
)

// ---- messages ----

type (
	textDeltaMsg   struct{ text string }
	textDoneMsg    struct{}
	toolStartMsg   struct {
		name string
		args json.RawMessage
	}
	toolEndMsg struct {
		name   string
		result string
		err    error
	}
	agentErrMsg    struct{ err error }
	agentDoneMsg   struct{ messages []provider.Message }
	modelsLoadedMsg struct {
		models []provider.Model
		err    error
	}
	permAskMsg struct{ req permRequest }
	tickMsg    time.Time
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayModels
	overlaySessions
	overlayCommands
	overlayPermission
)

type permRequest struct {
	action   string
	resource string
	command  string
	replyCh  chan string
}

type block struct {
	kind    string // user | assistant | streaming | tool_start | tool_end | error | info | footer
	label   string
	body    string
	pending bool
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg      *config.Config
	client   *provider.Client
	modelID  string
	provider string
	agent    *agent.Agent
	store    *session.Store
	sess     *session.Session

	width, height int
	sidebarOn     bool

	vp      viewport.Model
	ta      textarea.Model
	blocks  []block
	overlay overlayKind
	items   []string
	sel     int
	title   string
	footer  string
	perm    *permRequest

	streaming    bool
	streamBuf    strings.Builder
	spinFrame    int
	turnStart    time.Time
	turnCancel   context.CancelFunc
	userCancel   bool
	leaderPending bool
	knownModels  []provider.Model
	status       string
	quitting     bool

	ctx    context.Context
	cwd    string
}

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// New builds the TUI model.
func New(cfg *config.Config, cwd string) (*Model, error) {
	ta := textarea.New()
	ta.Placeholder = `Ask anything… "Fix a TODO in the codebase"`
	ta.Focus()
	ta.Prompt = ""
	ta.SetHeight(3)

	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured — run `opentitam setup` first")
	}
	provName, bare := pickModel(cfg)

	p := cfg.Providers[provName]
	m := &Model{
		cfg:      cfg,
		client:   provider.NewClient(provName, p.BaseURL, p.APIKey),
		modelID:  bare,
		provider: provName,
		store:    session.NewStore(),
		ctx:      context.Background(),
		cwd:      cwd,
		ta:       ta,
		vp:       viewport.New(80, 20),
		status:   "ready",
	}
	m.vp.Style = lipgloss.NewStyle().Padding(0, 1)

	m.agent = agent.New(m.client, bare, toolsFrom(cfg), permsFrom(cfg), cwd)
	m.agent.System = agent.SystemPrompt(cwd) + "\n\n" + agent.LoadInstructions(cwd)
	m.agent.SetApprover(m.AskApproval)

	m.sess = m.store.New(bare, provName)
	m.blocks = []block{{
		kind:  "info",
		label: "OpenTitam",
		body:  fmt.Sprintf("provider %s · model %s\nleader ctrl+x · ctrl+p commands · enter send", provName, orNone(bare)),
	}}
	return m, nil
}

func orNone(s string) string {
	if s == "" {
		return "(no model selected — ctrl+p → models)"
	}
	return s
}

// pickModel returns (provider, bareModel) from config; prefers configured model.
func pickModel(cfg *config.Config) (string, string) {
	if cfg.Model != "" {
		if name, rest, ok := strings.Cut(cfg.Model, "/"); ok {
			if _, ok := cfg.Providers[name]; ok {
				return name, rest
			}
			for pname := range cfg.Providers {
				if strings.HasPrefix(cfg.Model, pname+"/") {
					return pname, strings.TrimPrefix(cfg.Model, pname+"/")
				}
			}
			return name, rest
		}
	}
	for name := range cfg.Providers {
		return name, cfg.Model
	}
	return "", ""
}
