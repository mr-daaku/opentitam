package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/provider"
)

// Init starts the blink command.
func (m *Model) Init() tea.Cmd { return textarea.Blink }

// Update is the root message handler.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case textDeltaMsg:
		m.streamBuf.WriteString(msg.text)
		m.setStreamingText(m.streamBuf.String())
		return m, nil

	case textDoneMsg:
		body := m.streamBuf.String()
		m.streamBuf.Reset()
		if strings.TrimSpace(body) != "" {
			// Assistant footer: Build · model · duration (spec §4.2)
			dur := time.Since(m.turnStart).Seconds()
			m.blocks = append(m.blocks,
				block{kind: "assistant", label: "assistant", body: body},
				block{kind: "footer", body: fmt.Sprintf("Build · %s · %.1fs", m.modelID, dur)},
			)
		}
		return m, m.scrollToBottom()

	case toolStartMsg:
		m.blocks = append(m.blocks, block{kind: "tool_start", label: msg.name, body: compactArgs(msg.args), pending: true})
		return m, m.scrollToBottom()

	case toolEndMsg:
		for i := len(m.blocks) - 1; i >= 0; i-- {
			b := &m.blocks[i]
			if b.kind == "tool_start" && b.pending && b.label == msg.name {
				b.pending = false
				icon := "✓"
				if msg.err != nil {
					icon = "✗"
				}
				b.kind = "tool_end"
				b.body = icon + "\n" + collapse(msg.result, 10)
				break
			}
		}
		return m, m.scrollToBottom()

	case agentErrMsg:
		m.streaming = false
		if m.userCancel {
			m.blocks = append(m.blocks, block{kind: "info", label: "interrupted", body: "Session interrupted (esc)."})
			m.userCancel = false
		} else {
			m.blocks = append(m.blocks, block{kind: "error", label: "error", body: msg.err.Error()})
		}
		m.status = "ready"
		return m, m.scrollToBottom()

	case agentDoneMsg:
		m.streaming = false
		m.sess.Messages = msg.messages
		m.sess.SetTitleFromFirstUser()
		_ = m.store.Save(m.sess)
		m.status = "ready"
		return m, nil

	case modelsLoadedMsg:
		if msg.err != nil {
			m.blocks = append(m.blocks, block{kind: "error", label: "error", body: "models: " + msg.err.Error()})
			return m, m.scrollToBottom()
		}
		m.knownModels = msg.models
		m.items = m.items[:0]
		for _, mod := range msg.models {
			m.items = append(m.items, mod.ID)
		}
		m.overlay = overlayModels
		m.title = "Select model"
		m.footer = "↑↓ select · enter confirm · esc close"
		m.sel = 0
		return m, nil

	case permAskMsg:
		m.perm = &msg.req
		m.overlay = overlayPermission
		m.title = "Permission required"
		m.footer = "↑↓ select · enter confirm · esc reject"
		m.sel = 0
		return m, nil

	case tickMsg:
		if m.streaming {
			m.spinFrame = (m.spinFrame + 1) % len(spinFrames)
			return m, tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
		}
		return m, nil
	}
	return m, nil
}

// handleKey routes keys by overlay/leader state.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// Leader chord (ctrl+x …): one-shot next-key dispatch.
	if m.leaderPending {
		m.leaderPending = false
		switch k {
		case "q":
			m.quitting = true
			return m, tea.Quit
		case "m":
			return m, m.fetchModels()
		case "n":
			return m, m.newSession()
		case "l":
			return m, m.openSessions()
		case "c":
			return m, m.doCompact()
		case "b":
			m.sidebarOn = !m.sidebarOn
			return m, nil
		}
		return m, nil // unknown chord: swallow
	}

	// Permission overlay: full selection UI + y/a/n shortcuts.
	if m.overlay == overlayPermission {
		return m.handlePermKey(msg)
	}

	// Other overlays.
	if m.overlay != overlayNone {
		switch k {
		case "esc", "ctrl+c":
			m.overlay = overlayNone
			return m, nil
		case "up", "ctrl+k":
			if m.sel > 0 {
				m.sel--
			}
			return m, nil
		case "down", "ctrl+j":
			if m.sel < len(m.items)-1 {
				m.sel++
			}
			return m, nil
		case "pgup":
			m.sel -= 10
			if m.sel < 0 {
				m.sel = 0
			}
			return m, nil
		case "pgdown":
			m.sel += 10
			if m.sel >= len(m.items) {
				m.sel = maxInt(0, len(m.items)-1)
			}
			return m, nil
		case "home":
			m.sel = 0
			return m, nil
		case "end":
			m.sel = maxInt(0, len(m.items)-1)
			return m, nil
		case "enter":
			return m, m.selectItem()
		}
		// Slash-command popup: typing filters (spec §8.6).
		if m.overlay == overlayCommands && msg.Type == tea.KeyRunes {
			m.ta.SetValue(m.ta.Value() + string(msg.Runes))
			m.items = filterSlash(m.ta.Value())
			m.sel = 0
			return m, nil
		}
		return m, nil
	}

	// Global keys (spec §3).
	switch k {
	case "ctrl+c":
		// Ctrl+C: clear prompt when input non-empty (spec prompt.clear),
		// otherwise interrupt/exit.
		if m.ta.Value() != "" && !m.streaming {
			m.ta.Reset()
			return m, nil
		}
		if m.streaming {
			return m, m.interrupt()
		}
		m.quitting = true
		return m, tea.Quit
	case "ctrl+d":
		m.quitting = true
		return m, tea.Quit
	case "ctrl+x":
		m.leaderPending = true
		return m, nil
	case "ctrl+p":
		// Command palette (spec command_list).
		m.overlay = overlayCommands
		m.title = "Commands"
		m.footer = "type to filter · enter run · esc close"
		m.items = allCommands()
		m.sel = 0
		return m, nil
	case "ctrl+o":
		return m, m.openSessions()
	case "esc":
		if m.streaming {
			return m, m.interrupt()
		}
		return m, nil
	case "f2":
		return m, m.cycleModel(1)
	case "shift+f2":
		return m, m.cycleModel(-1)
	case "pgup":
		m.vp.HalfViewUp()
		return m, nil
	case "pgdown":
		m.vp.HalfViewDown()
		return m, nil
	}

	// Enter submits; shift/ctrl/alt+enter (and ctrl+j) insert newline (spec §3.4).
	if msg.Type == tea.KeyEnter && !msg.Alt && k != "ctrl+enter" {
		return m.submit()
	}
	if k == "ctrl+j" {
		m.ta.InsertRune('\n')
		return m, nil
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)

	// Opening "/" triggers the slash popup (autocomplete spec §8.6).
	if m.ta.Value() == "/" {
		m.overlay = overlayCommands
		m.title = "Commands"
		m.footer = "type to filter · enter select · esc close"
		m.items = filterSlash("/")
		m.sel = 0
	}
	return m, cmd
}

func (m *Model) handlePermKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := []string{"Allow once", "Always allow", "Reject"}
	switch msg.String() {
	case "up", "left", "ctrl+k":
		if m.sel > 0 {
			m.sel--
		}
	case "down", "right", "ctrl+j":
		if m.sel < len(opts)-1 {
			m.sel++
		}
	case "enter":
		m.replyPerm([]string{"allow", "allow_always", "reject"}[m.sel])
	case "y", "Y":
		m.replyPerm("allow")
	case "a", "A":
		m.replyPerm("allow_always")
	case "n", "N", "esc":
		m.replyPerm("reject")
	}
	return m, nil
}

func (m *Model) replyPerm(decision string) {
	if m.perm != nil {
		m.perm.replyCh <- decision
		m.perm = nil
	}
	m.overlay = overlayNone
}

// ---- actions ----

func (m *Model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.ta.Value())
	if text == "" || m.streaming {
		return m, nil
	}
	m.ta.Reset()
	m.overlay = overlayNone

	// Slash commands run locally.
	if strings.HasPrefix(text, "/") {
		if handled, cmd := m.runSlash(text); handled {
			return m, cmd
		}
	}

	if m.modelID == "" {
		m.blocks = append(m.blocks, block{kind: "error", label: "error", body: "No model selected. Press ctrl+p → models (or leader x m)."})
		return m, m.scrollToBottom()
	}

	m.blocks = append(m.blocks, block{kind: "user", label: "you", body: text})
	m.streaming = true
	m.streamBuf.Reset()
	m.turnStart = time.Now()
	m.status = "thinking"
	m.blocks = append(m.blocks, block{kind: "streaming", label: "assistant", body: ""})

	history := append([]provider.Message{}, m.sess.Messages...)
	prompt := text
	ctx := m.ctx
	cctx, cancel := context.WithCancel(ctx)
	m.turnCancel = cancel

	cmd := tea.Batch(
		tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
		func() tea.Msg {
			for ev := range m.agent.Run(cctx, history, prompt) {
				switch ev.Kind {
				case "text_delta":
					send(textDeltaMsg{ev.Text})
				case "text_done":
					send(textDoneMsg{})
				case "tool_start":
					send(toolStartMsg{ev.ToolName, ev.ToolArgs})
				case "tool_end":
					send(toolEndMsg{ev.ToolName, ev.Result, ev.Err})
				case "error":
					send(agentErrMsg{ev.Err})
				case "done":
					var msgs []provider.Message
					_ = json.Unmarshal([]byte(ev.Result), &msgs)
					send(agentDoneMsg{msgs})
				}
			}
			return nil
		},
	)
	return m, cmd
}

func (m *Model) interrupt() tea.Cmd {
	if m.turnCancel != nil {
		m.userCancel = true
		m.turnCancel()
	}
	return nil
}

func (m *Model) newSession() tea.Cmd {
	m.sess = m.store.New(m.modelID, m.provider)
	m.blocks = []block{{kind: "info", label: "OpenTitam", body: "New session."}}
	return nil
}

func (m *Model) openSessions() tea.Cmd {
	m.overlay = overlaySessions
	m.title = "Sessions"
	m.footer = "enter open · esc close"
	m.items = sessionItems(m.store, m.sess.ID)
	m.sel = 0
	return nil
}

func (m *Model) fetchModels() tea.Cmd {
	if m.streaming {
		return nil
	}
	client := m.client
	ctx := m.ctx
	m.status = "loading models…"
	return func() tea.Msg {
		models, err := client.ListModels(ctx)
		return modelsLoadedMsg{models: models, err: err}
	}
}

func (m *Model) cycleModel(dir int) tea.Cmd {
	// Uses cached list; triggers a fetch when empty.
	if len(m.knownModels) == 0 {
		return m.fetchModels()
	}
	idx := 0
	for i, mod := range m.knownModels {
		if mod.ID == m.modelID {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(m.knownModels)) % len(m.knownModels)
	m.applyModel(m.knownModels[idx].ID)
	return nil
}

func (m *Model) applyModel(id string) {
	m.modelID = id
	m.cfg.Model = m.provider + "/" + id
	_ = config.SaveGlobal(m.cfg)
	m.agent.Model = id
	m.sess.Model = id
	m.blocks = append(m.blocks, block{kind: "info", label: "model", body: "switched to " + m.provider + "/" + id})
}

func (m *Model) doCompact() tea.Cmd {
	m.blocks = append(m.blocks, block{kind: "info", label: "compact", body: "Context auto-trims at 120k estimated tokens before the next message."})
	return nil
}

func (m *Model) selectItem() tea.Cmd {
	if m.sel < 0 || m.sel >= len(m.items) {
		m.overlay = overlayNone
		return nil
	}
	item := m.items[m.sel]
	switch m.overlay {
	case overlayModels:
		m.applyModel(item)
		m.overlay = overlayNone
	case overlaySessions:
		m.overlay = overlayNone
		if item == "+ New session" {
			m.newSession()
		} else if id := sessionIDFromItem(item); id != "" {
			if s, err := m.store.Load(id); err == nil {
				m.sess = s
				m.blocks = []block{{kind: "info", label: "session", body: "Resumed · " + s.Title}}
				for _, msg := range s.Messages {
					switch msg.Role {
					case "user":
						m.blocks = append(m.blocks, block{kind: "user", label: "you", body: msg.Content})
					case "assistant":
						if msg.Content != "" {
							m.blocks = append(m.blocks, block{kind: "assistant", label: "assistant", body: msg.Content})
						}
					}
				}
			}
		}
	case overlayCommands:
		m.overlay = overlayNone
		m.ta.Reset()
		return m.runCommand(item)
	}
	return nil
}

func (m *Model) runSlash(text string) (bool, tea.Cmd) {
	cmd := strings.Fields(text)[0]
	switch cmd {
	case "/exit", "/quit", "/q":
		m.quitting = true
		return true, tea.Quit
	case "/clear":
		m.blocks = nil
		return true, nil
	case "/new":
		m.newSession()
		return true, nil
	case "/models":
		return true, m.fetchModels()
	case "/sessions":
		return true, m.openSessions()
	case "/compact":
		m.doCompact()
		return true, nil
	case "/help":
		m.blocks = append(m.blocks, block{kind: "info", label: "help", body:
			"Commands: /models /sessions /new /compact /clear /help /exit\n" +
				"Keys: enter send · shift+enter newline · ctrl+p commands · ctrl+o sessions · ctrl+x leader (q exit, m models, n new, l list, b sidebar) · f2 cycle model · esc interrupt"})
		return true, m.scrollToBottom()
	}
	return false, nil
}

// runCommand executes a command-palette selection by label prefix.
func (m *Model) runCommand(label string) tea.Cmd {
	name := strings.Fields(label)[0]
	return m.runSlashCmd(name)
}

func (m *Model) runSlashCmd(name string) tea.Cmd {
	switch name {
	case "/exit":
		m.quitting = true
		return tea.Quit
	case "/clear":
		m.blocks = nil
	case "/new":
		m.newSession()
	case "/models":
		return m.fetchModels()
	case "/sessions":
		return m.openSessions()
	case "/compact":
		m.doCompact()
	case "/help":
		m.blocks = append(m.blocks, block{kind: "info", label: "help", body: "See /help in the composer for the full list."})
	}
	return nil
}

// ---- streaming transcript ----

func (m *Model) setStreamingText(text string) {
	if n := len(m.blocks); n > 0 && m.blocks[n-1].kind == "streaming" {
		m.blocks[n-1].body = text
	} else {
		m.blocks = append(m.blocks, block{kind: "streaming", label: "assistant", body: text})
	}
	m.renderTranscript()
	m.vp.GotoBottom()
}

// AskApproval blocks the agent goroutine until the UI replies.
func (m *Model) AskApproval(action, resource, command string) string {
	replyCh := make(chan string, 1)
	send(permAskMsg{permRequest{action: action, resource: resource, command: command, replyCh: replyCh}})
	return <-replyCh
}

// send delivers a message from goroutines.
var prog *tea.Program

func send(msg tea.Msg) {
	if prog != nil {
		prog.Send(msg)
	}
}

// Run starts the TUI (alt screen).
func Run(cfg *config.Config, cwd string) error {
	m, err := New(cfg, cwd)
	if err != nil {
		return err
	}
	prog = tea.NewProgram(m, tea.WithAltScreen())
	_, err = prog.Run()
	return err
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
