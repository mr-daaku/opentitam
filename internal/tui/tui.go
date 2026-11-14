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
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mr-daaku/opentitam/internal/agent"
	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/session"
)

// Version is the single source of truth for the build version (cli prints it
// too — cli.Version references this constant).
const Version = "0.3.0"

// ---- theme: exact hexes from the OpenCode "opencode" dark theme ----

var (
	// surfaces
	bgBase   = lipgloss.Color("#0a0a0a")
	bgRaised = lipgloss.Color("#141414")
	bgHigh   = lipgloss.Color("#1e1e1e")
	// text
	textBase  = lipgloss.Color("#eeeeee")
	textMuted = lipgloss.Color("#808080")
	// chrome
	borderBase   = lipgloss.Color("#484848")
	borderActive = lipgloss.Color("#606060")
	// feedback
	colError   = lipgloss.Color("#e06c75")
	colWarning = lipgloss.Color("#f5a742")
	colSuccess = lipgloss.Color("#7fd88f")
	colInfo    = lipgloss.Color("#56b6c2")
	// hues
	colAccent      = lipgloss.Color("#9d7cd8") // purple, headings
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

	// markdown renderer styles (chat text is colorful, never raw ** etc).
	styleMDH1     = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleMDH2     = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleMDH3     = lipgloss.NewStyle().Bold(true).Foreground(colInteractive)
	styleMDBold   = lipgloss.NewStyle().Bold(true).Foreground(textBase)
	styleMDItalic = lipgloss.NewStyle().Italic(true).Foreground(colAccent)
	styleMDCode   = lipgloss.NewStyle().Foreground(colInfo).Background(bgHigh)
	styleMDCodeBl = lipgloss.NewStyle().Foreground(colInfo).Background(bgHigh).Padding(0, 1)
	styleMDBullet = lipgloss.NewStyle().Foreground(colInteractive)
	styleMDLink   = lipgloss.NewStyle().Foreground(colInfo).Underline(true)
	styleMDQuote  = lipgloss.NewStyle().Foreground(textMuted).Italic(true)

	// thinking (reasoning) block: hidden by default, click to expand.
	styleThinkIdle = lipgloss.NewStyle().Foreground(textMuted).Faint(true)
	styleThinkHead = lipgloss.NewStyle().Bold(true).Foreground(colInfo)
	styleThinkBody = lipgloss.NewStyle().Foreground(textMuted).Faint(true)

	// sidebar section headers.
	styleSideHead = lipgloss.NewStyle().Bold(true).Foreground(colInteractive)

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
	reasonDeltaMsg struct{ text string }
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
	agentErrMsg     struct{ err error }
	agentDoneMsg    struct{ messages []provider.Message }
	modelsLoadedMsg struct {
		models []provider.Model
		err    error
	}
	permAskMsg struct{ req permRequest }
	apiTestMsg struct {
		preset  string
		baseURL string
		key     string
		res     provider.TestResult
		err     error
	}
	tickMsg time.Time
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayModels
	overlaySessions
	overlayCommands
	overlayPermission
	overlayProvider // first-run: pick a provider preset
	overlayKey      // paste API key / custom base URL
)

type permRequest struct {
	action   string
	resource string
	command  string
	replyCh  chan string
}

type block struct {
	kind    string // user | assistant | streaming | thinking | tool_start | tool_end | error | info | footer
	label   string
	body    string
	pending bool
	// expanded toggles the thinking block (collapsed by default, click shows).
	expanded bool
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

	// command palette / slash popup filter buffer (never pollutes the composer)
	filter string

	// transcript hit-testing (click on thinking blocks) + viewport rect
	blockOfLine []int
	vpTop       int
	vpH         int
	vpX0        int
	vpW         int

	// current-turn reasoning block index (-1 = none yet)
	thinkIdx  int
	reasonBuf strings.Builder

	// sidebar folder details (cached on layout)
	folder folderInfo

	streaming     bool
	streamBuf     strings.Builder
	turnHadText   bool
	spinFrame     int
	turnStart     time.Time
	turnCancel    context.CancelFunc
	userCancel    bool
	leaderPending bool
	knownModels   []provider.Model
	status        string
	quitting      bool

	// first-run provider setup
	keyInput    textarea.Model
	setupPreset provider.Preset
	setupBase   string
	setupMode   string // "base" (custom URL entry) | "key"
	setupErr    string
	testing     bool

	ctx context.Context
	cwd string
}

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// folderInfo is the cached sidebar folder summary (path, counts, top entries).
type folderInfo struct {
	path    string
	dirs    int
	files   int
	entries []string // display names, directories first (max 10)
	err     bool
}

// readFolder inspects cwd for the right sidebar (folder details).
func readFolder(path string) folderInfo {
	fi := folderInfo{path: path}
	ents, err := os.ReadDir(path)
	if err != nil {
		fi.err = true
		return fi
	}
	var dirs, files []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, "▸ "+e.Name())
		} else {
			files = append(files, "· "+e.Name())
		}
	}
	fi.dirs, fi.files = len(dirs), len(files)
	for _, n := range dirs {
		if len(fi.entries) >= 10 {
			break
		}
		fi.entries = append(fi.entries, n)
	}
	for _, n := range files {
		if len(fi.entries) >= 10 {
			break
		}
		fi.entries = append(fi.entries, n)
	}
	return fi
}

// New builds the TUI model. With no provider configured the TUI still opens
// directly — it just starts in the first-run provider setup overlay.
func New(cfg *config.Config, cwd string) (*Model, error) {
	ta := textarea.New()
	ta.Placeholder = `Ask anything… "Fix a TODO in the codebase"`
	ta.Prompt = ""
	ta.ShowLineNumbers = false // no stray "1" gutter in the composer
	ta.Focus()
	ta.SetHeight(3)

	keyInput := textarea.New()
	keyInput.Prompt = ""
	keyInput.ShowLineNumbers = false
	keyInput.Placeholder = "paste your API key, then press enter"
	keyInput.SetWidth(64)
	keyInput.SetHeight(1)

	m := &Model{
		cfg:       cfg,
		store:     session.NewStore(),
		ctx:       context.Background(),
		cwd:       cwd,
		ta:        ta,
		keyInput:  keyInput,
		vp:        viewport.New(80, 20),
		status:    "ready",
		sidebarOn: true, // right rail on by default (auto-hides under 80 cols)
		thinkIdx:  -1,
	}
	m.vp.Style = lipgloss.NewStyle().Padding(0, 1)
	m.sess = m.store.New("", "")
	m.folder = readFolder(cwd)
	m.syncFocus()

	if len(cfg.Providers) == 0 {
		// First run: open straight into provider setup (no forced CLI wizard).
		m.blocks = []block{{
			kind:  "info",
			label: "OpenTitam",
			body:  "First run — pick a provider below. Only an API key is needed;\nthe base URL is stored locally and models load from its /models API.",
		}}
		m.beginSetup()
		return m, nil
	}

	provName, bare := pickModel(cfg)
	p := cfg.Providers[provName]
	m.client = provider.NewClient(provName, p.BaseURL, p.APIKey)
	m.modelID = bare
	m.provider = provName

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

// beginSetup opens the provider picker (first run or /api).
func (m *Model) beginSetup() {
	m.overlay = overlayProvider
	m.title = "Select a provider"
	m.footer = "↑↓ select · enter confirm · esc close"
	m.items = providerLabels()
	m.sel = 0
	m.setupErr = ""
	m.testing = false
	m.syncFocus()
}

// syncFocus keeps textarea focus in lockstep with the active overlay.
// bubbles' textarea.Update is a no-op while blurred — that is why typing or
// pasting into the API-key dialog used to do nothing. Returns the cursor-blink
// command for the newly focused input (nil when focus did not change).
func (m *Model) syncFocus() tea.Cmd {
	if m.overlay == overlayKey {
		if m.keyInput.Focused() {
			return nil
		}
		m.ta.Blur()
		return m.keyInput.Focus()
	}
	m.keyInput.Blur()
	if m.overlay == overlayNone {
		if m.ta.Focused() {
			return nil
		}
		return m.ta.Focus()
	}
	m.ta.Blur()
	return nil
}

func providerLabels() []string {
	out := make([]string, 0, 12)
	for _, p := range provider.Presets() {
		label := p.Label
		if !p.NeedsKey {
			label += "  · no key"
		}
		out = append(out, label)
	}
	return out
}

// applyProvider persists a TESTED provider (models list + default model) and
// rewires the live client/agent. Called only after a successful Test.
func (m *Model) applyProvider(preset, base, key string, res provider.TestResult) error {
	ids := make([]string, 0, len(res.Models))
	for _, mod := range res.Models {
		ids = append(ids, mod.ID)
	}
	def := res.WorkingModel
	if def == "" && len(ids) > 0 {
		def = ids[0]
	}
	cfg := m.cfg
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}
	cfg.Providers[preset] = config.Provider{BaseURL: base, APIKey: key, Models: ids}
	cfg.Model = preset + "/" + def
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}

	m.provider = preset
	m.modelID = def
	m.client = provider.NewClient(preset, base, key)
	m.agent = agent.New(m.client, def, toolsFrom(cfg), permsFrom(cfg), m.cwd)
	m.agent.System = agent.SystemPrompt(m.cwd) + "\n\n" + agent.LoadInstructions(m.cwd)
	m.agent.SetApprover(m.AskApproval)
	m.sess = m.store.New(def, preset)

	m.blocks = []block{{
		kind:  "info",
		label: "OpenTitam",
		body: fmt.Sprintf("Connected to %s — %d model(s), default %s\nleader ctrl+x · ctrl+p commands · enter send",
			preset, len(ids), def),
	}}

	// Jump straight into the model picker so the user chooses what to use.
	m.knownModels = res.Models
	m.overlay = overlayModels
	m.title = "Select model"
	m.footer = "↑↓ select · enter confirm · esc close"
	m.items = ids
	m.sel = 0
	for i, id := range ids {
		if id == def {
			m.sel = i
			break
		}
	}
	return nil
}

// runAPITest tests credentials off the UI thread; result arrives as apiTestMsg.
func (m *Model) runAPITest(preset, base, key string) tea.Cmd {
	return func() tea.Msg {
		c := provider.NewClient(preset, base, key)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		res, err := c.Test(ctx)
		return apiTestMsg{preset: preset, baseURL: base, key: key, res: res, err: err}
	}
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
