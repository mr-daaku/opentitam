package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/permissions"
	"github.com/mr-daaku/opentitam/internal/provider"
	"github.com/mr-daaku/opentitam/internal/session"
	"github.com/mr-daaku/opentitam/internal/tools"
)

func toolsFrom(cfg *config.Config) *tools.Registry { return tools.DefaultRegistry(cfg.Shell) }
func permsFrom(cfg *config.Config) *permissions.Engine {
	return permissions.New(cfg.Permissions)
}

// layout sizes transcript/editor around chrome.
func (m *Model) layout() {
	editorH := m.ta.Height() + 1 // rail row + input
	permH := 0
	if m.overlay == overlayPermission {
		permH = 7
	}
	transcriptH := m.height - 1 - editorH - permH - 1 // strip + composer + footer
	if transcriptH < 4 {
		transcriptH = 4
	}
	w := m.width
	if m.sidebarOn && w > 80 {
		w -= 42 // sidebar is exactly 42 cols
	}
	m.vp = viewport.New(maxInt(10, w), transcriptH)
	m.vp.Style = lipgloss.NewStyle().Padding(0, 1)
	m.ta.SetWidth(maxInt(20, m.width-6))

	// viewport rect (screen coords) — used for mouse hit-testing.
	m.vpTop = 1
	m.vpH = transcriptH
	m.vpX0 = 0
	m.vpW = w

	m.folder = readFolder(m.cwd)
	m.renderTranscript()
	m.vp.GotoBottom()
}

// View renders the full screen (spec §1 layout).
func (m *Model) View() string {
	if m.quitting {
		return "\n"
	}

	// 1. Top strip: session tabs row (1 line, raised bg).
	title := m.sess.Title
	if title == "" || title == "New session" {
		title = "New session"
	}
	tab := " " + truncate(title, 40) + " "
	right := fmt.Sprintf("%s/%s ", m.provider, orNone(m.modelID))
	leftW := lipgloss.Width(tab)
	rightW := lipgloss.Width(right)
	pad := m.width - leftW - rightW
	if pad < 1 {
		pad = 1
	}
	strip := lipgloss.NewStyle().Background(bgRaised).Width(m.width).Render(
		lipgloss.NewStyle().Background(bgRaised).Foreground(colInteractive).Bold(true).Render(tab) +
			lipgloss.NewStyle().Background(bgRaised).Foreground(textMuted).Render(strings.Repeat(" ", pad)) +
			lipgloss.NewStyle().Background(bgRaised).Foreground(textMuted).Render(right),
	)

	// 2. Body row: transcript (left) + optional sidebar (right).
	transcript := m.vp.View()
	body := transcript
	if m.sidebarOn && m.width > 80 {
		side := m.renderSidebar()
		body = lipgloss.JoinHorizontal(lipgloss.Top, transcript, side)
	}

	// 3. Permission stack (above composer, spec §8.4).
	permBlock := ""
	if m.overlay == overlayPermission && m.perm != nil {
		permBlock = m.renderPermission()
	}

	// 4. Composer: ╹ rail + input (spec §6).
	rail := styleRail.Render("╹")
	composerInner := lipgloss.JoinHorizontal(lipgloss.Top,
		rail+" ",
		m.ta.View(),
	)
	composer := styleComposer.Render(composerInner)

	// 5. Composer footer: context% left, key hints right (spec §6).
	ctxTokens, ctxPct := estimateContext(m.sess.Messages)
	leftFoot := fmt.Sprintf("%s (%d%%)", commaify(ctxTokens), ctxPct)
	rightFoot := "⇧tab agents  ctrl+p commands"
	if m.streaming {
		rightFoot = "esc interrupt  " + rightFoot
	}
	footPad := m.width - lipgloss.Width(leftFoot) - lipgloss.Width(rightFoot) - 2
	if footPad < 1 {
		footPad = 1
	}
	footer := lipgloss.JoinHorizontal(lipgloss.Top,
		styleFooterHint.Render(leftFoot),
		strings.Repeat(" ", footPad),
		styleFooterHint.Render(rightFoot),
	)

	out := lipgloss.JoinVertical(lipgloss.Left, strip, body, permBlock, composer, footer)

	// 6. Modal overlays render on a PURE BLACK screen — transcript text can
	// never bleed through or mis-align behind a dialog.
	switch m.overlay {
	case overlayModels, overlaySessions, overlayCommands, overlayProvider:
		out = m.renderDialog()
	case overlayKey:
		out = m.renderSetupKey()
	case overlayPermission:
		out = m.centerOverlay(m.renderPermission())
	}
	return out
}

// renderSidebar is the right-hand info rail: session title, model, context,
// folder details and the session list — everything the user should see.
func (m *Model) renderSidebar() string {
	var rows []string

	// Header + version.
	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			styleAccent.Render(" ⌬ OpenTitam"),
			styleMuted.Render("  v"+Version),
		), "")

	// Session.
	rows = append(rows, styleSideHead.Render(" ─ session "))
	title := m.sess.Title
	if title == "" {
		title = "New session"
	}
	for _, l := range wrapText(title, 40) {
		rows = append(rows, " "+l)
	}
	ctxTok, ctxPct := estimateContext(m.sess.Messages)
	rows = append(rows, styleMuted.Render(fmt.Sprintf(" %d msgs · %s tok · %d%%",
		len(m.sess.Messages), commaify(ctxTok), ctxPct)))
	rows = append(rows, "")

	// Model / provider.
	rows = append(rows, styleSideHead.Render(" ─ model "))
	rows = append(rows, " "+truncate(m.provider, 40))
	rows = append(rows, styleMuted.Render(" "+truncate(orNone(m.modelID), 40)))
	rows = append(rows, "")

	// Folder details.
	rows = append(rows, styleSideHead.Render(" ─ folder "))
	if m.folder.err {
		rows = append(rows, styleError.Render("  (unreadable)"))
	} else {
		for _, l := range wrapText(m.folder.path, 40) {
			rows = append(rows, styleMuted.Render(" "+l))
		}
		rows = append(rows, fmt.Sprintf(" %s%d dirs · %d files",
			styleMDBullet.Render("•"), m.folder.dirs, m.folder.files))
		for _, e := range m.folder.entries {
			rows = append(rows, styleMuted.Render("  "+truncate(e, 39)))
		}
		if m.folder.dirs+m.folder.files > len(m.folder.entries) {
			rows = append(rows, styleMuted.Render(fmt.Sprintf("  … %d more",
				m.folder.dirs+m.folder.files-len(m.folder.entries))))
		}
	}
	rows = append(rows, "")

	// Sessions.
	rows = append(rows, styleSideHead.Render(" ─ sessions "))
	items := sessionItems(m.store, m.sess.ID)
	if len(items) == 0 {
		rows = append(rows, styleMuted.Render("  (none)"))
	}
	for _, it := range items {
		line := truncate(it, 39)
		if strings.Contains(it, m.sess.ID) && m.sess.ID != "" {
			rows = append(rows, styleSideSel.Render("┃"+line))
		} else {
			rows = append(rows, styleMuted.Render("  "+line))
		}
	}

	rows = append(rows, "", styleMuted.Render("  ctrl+x b hide · ctrl+p cmds"))

	h := m.vpH
	if h < 4 {
		h = 4
	}
	// Never grow past the transcript height (keeps the screen from overflowing).
	if len(rows) > h {
		rows = rows[:h]
	}

	content := strings.Join(rows, "\n")
	return styleSidebar.Height(h).Render(content)
}

// renderPermission is the bottom-stack approval prompt (spec §8.4).
func (m *Model) renderPermission() string {
	p := m.perm
	warn := styleWarn.Render("△") + " " + lipgloss.NewStyle().Bold(true).Foreground(textBase).Render("Permission required")
	body := fmt.Sprintf("%s  %s", styleToolLabel.Render(p.action), p.resource)
	if p.command != "" {
		body += "\n" + styleMuted.Render("$ "+truncate(p.command, m.width-8))
	}
	opts := []string{"Allow once", "Always allow", "Reject"}
	var optRows []string
	for i, o := range opts {
		if i == m.sel {
			optRows = append(optRows, styleSelRow.Render("┃ "+o))
		} else {
			optRows = append(optRows, "  "+o)
		}
	}
	hints := styleFooterHint.Render("⇆ select · enter confirm · esc reject")
	panel := lipgloss.JoinVertical(lipgloss.Left,
		warn, "", body, "",
		strings.Join(optRows, "\n"), "",
		hints,
	)
	return lipgloss.NewStyle().
		Background(bgRaised).
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(borderBase).
		Width(minInt(m.width, 72)).
		Padding(0, 2).
		Render(panel)
}

// renderDialog centers a DialogSelect-style panel (spec §8, widths 60/88)
// on a pure black full-screen backdrop.
func (m *Model) renderDialog() string {
	width := 60
	if m.overlay == overlayModels || m.overlay == overlaySessions {
		width = 88
	}
	if width > m.width-2 {
		width = maxInt(20, m.width-2)
	}

	var rows []string
	rows = append(rows, styleDialogTitle.Render(m.title), "")
	maxShow := m.height - 8
	if maxShow < 3 {
		maxShow = 3
	}
	start := 0
	if m.sel >= maxShow {
		start = m.sel - maxShow + 1
	}
	end := start + maxShow
	if end > len(m.items) {
		end = len(m.items)
	}
	if len(m.items) == 0 {
		rows = append(rows, styleMuted.Render("No items available"))
	}
	for i := start; i < end; i++ {
		it := m.items[i]
		if i == m.sel {
			rows = append(rows, styleSelRow.Render("┃ "+truncate(it, width-8)))
		} else {
			marker := "  "
			if m.overlay == overlayModels && m.provider+"/"+it == m.cfg.Model {
				marker = "▪ "
			}
			rows = append(rows, marker+truncate(it, width-8))
		}
	}
	rows = append(rows, "", styleFooterHint.Render(m.footer))

	panel := styleDialog.
		Width(width).
		Render(strings.Join(rows, "\n"))

	return m.centerOverlay(panel)
}

// renderSetupKey is the API-key / custom base-URL input dialog (first-run setup).
func (m *Model) renderSetupKey() string {
	width := 64
	if width > m.width-2 {
		width = maxInt(20, m.width-2)
	}
	title := "API key"
	sub := m.setupPreset.Label
	if m.setupMode == "base" {
		title = "Base URL"
		sub = m.setupPreset.Label + " — enter the OpenAI-compatible endpoint"
	} else if m.setupBase != "" {
		sub += " · " + m.setupBase
	}
	m.keyInput.SetWidth(maxInt(20, width-8))

	rows := []string{
		styleDialogTitle.Render(title),
		styleMuted.Render(truncate(sub, width-8)),
		"",
		m.keyInput.View(),
	}
	if m.setupErr != "" {
		rows = append(rows, "", styleError.Render("✗ "+truncate(m.setupErr, width-8)))
	}
	if m.testing {
		sp := spinFrames[m.spinFrame%len(spinFrames)]
		rows = append(rows, "", styleMuted.Render(sp+" testing endpoint + chat — nothing is saved until it passes"))
	} else {
		hint := "enter confirm · esc back"
		if len(m.cfg.Providers) == 0 {
			hint = "enter confirm · esc back (setup required on first run)"
		}
		rows = append(rows, "", styleFooterHint.Render(hint))
	}

	panel := styleDialog.
		Width(width).
		Render(strings.Join(rows, "\n"))

	return m.centerOverlay(panel)
}

// centerOverlay places the panel dead-center on a PURE BLACK screen so no
// transcript text can show through, mis-aligned or otherwise.
func (m *Model) centerOverlay(panel string) string {
	w, h := m.width, m.height
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, panel,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(bgBase),
	)
}

// renderTranscript flattens blocks into viewport content (spec §4, §5) and
// records which block each rendered line belongs to (mouse hit-testing).
func (m *Model) renderTranscript() {
	var lines []string
	var owner []int
	add := func(bi int, s string) {
		for _, l := range strings.Split(s, "\n") {
			lines = append(lines, l)
			owner = append(owner, bi)
		}
	}

	for bi, blk := range m.blocks {
		switch blk.kind {
		case "user":
			// User message: ┃ left border + raised bg (spec §4.1).
			for i, line := range strings.Split(blk.body, "\n") {
				if i == 0 {
					add(bi, styleUserBody.Render("┃ "+line))
				} else {
					add(bi, styleUserBody.Render("  "+line))
				}
			}
			add(bi, "")
		case "assistant":
			add(bi, indent(renderMarkdown(blk.body), "  "))
			add(bi, "")
		case "streaming":
			sp := spinFrames[m.spinFrame%len(spinFrames)]
			head := styleMuted.Render("  " + sp + " ")
			body := indent(renderMarkdown(blk.body), "  ")
			first, rest, _ := strings.Cut(body, "\n")
			add(bi, head+first)
			if rest != "" {
				for _, l := range strings.Split(rest, "\n") {
					add(bi, l)
				}
			}
			add(bi, "")
		case "thinking":
			// Hidden by default — one dim line; click expands (spec: think
			// shows only on click).
			if blk.expanded {
				add(bi, styleThinkHead.Render("  ▾ thinking"))
				for _, l := range strings.Split(blk.body, "\n") {
					add(bi, styleThinkBody.Render("    "+truncate(l, 200)))
				}
				add(bi, "")
			} else {
				n := len(strings.Split(blk.body, "\n"))
				add(bi, styleThinkIdle.Render(
					fmt.Sprintf("  ▸ thinking · %d line(s) · click to expand", n)))
				add(bi, "")
			}
		case "footer":
			add(bi, styleMuted.Render("  "+blk.body))
			add(bi, "")
		case "tool_start":
			icon := styleMuted.Render("⠋ ")
			if !blk.pending {
				icon = styleToolOK.Render("✓ ")
			}
			add(bi, "  "+icon+styleToolLabel.Render(blk.label)+" "+styleMuted.Render(truncate(blk.body, 100)))
			add(bi, "")
		case "tool_end":
			icon := styleToolOK.Render("✓")
			if strings.HasPrefix(blk.body, "✗") {
				icon = styleToolErr.Render("✗")
			}
			lines2 := strings.SplitN(blk.body, "\n", 2)
			add(bi, "  "+icon+" "+styleToolLabel.Render(blk.label))
			if len(lines2) > 1 && strings.TrimSpace(lines2[1]) != "" {
				add(bi, indent(lines2[1], "    "))
			}
			add(bi, "")
		case "error":
			add(bi, "  "+styleError.Render("✗ "+blk.label))
			add(bi, indent(blk.body, "    "))
			add(bi, "")
		case "info":
			add(bi, styleMuted.Render("  · "+blk.label))
			add(bi, indent(blk.body, "    "))
			add(bi, "")
		}
	}
	m.blockOfLine = owner
	m.vp.SetContent(strings.Join(lines, "\n"))
}

func (m *Model) scrollToBottom() tea.Cmd {
	m.renderTranscript()
	m.vp.GotoBottom()
	return nil
}

// ---- helpers ----

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func collapse(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append(lines[:n], fmt.Sprintf("… %d more lines", len(lines)-n))
	}
	for i, l := range lines {
		if len(l) > 200 {
			lines[i] = l[:200] + "…"
		}
	}
	return strings.Join(lines, "\n")
}

func compactArgs(raw []byte) string {
	var mp map[string]any
	if json.Unmarshal(raw, &mp) != nil {
		return string(raw)
	}
	for _, k := range []string{"path", "pattern", "command"} {
		if v, ok := mp[k].(string); ok {
			return v
		}
	}
	return string(raw)
}

func sessionItems(store *session.Store, currentID string) []string {
	items := []string{"+ New session"}
	for _, s := range store.List() {
		items = append(items, fmt.Sprintf("%s  ·  %s", truncate(s.Title, 50), s.ID))
	}
	return items
}

func sessionIDFromItem(item string) string {
	fields := strings.Fields(item)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// filterCommands filters the palette list: "" shows every command with its
// key hints; a typed prefix matches bare command names (with or without "/").
func filterCommands(prefix string) []string {
	if prefix == "" {
		return allCommands()
	}
	want := strings.TrimPrefix(prefix, "/")
	var out []string
	for _, c := range slashList() {
		name := strings.TrimPrefix(c, "/")
		if strings.HasPrefix(c, prefix) || strings.HasPrefix(name, want) {
			out = append(out, c)
		}
	}
	return out
}

func slashList() []string {
	return []string{
		"/api", "/clear", "/compact", "/exit", "/help", "/models", "/new", "/sessions",
	}
}

func allCommands() []string {
	return []string{
		"/new      ctrl+x n",
		"/sessions ctrl+x l",
		"/models   ctrl+x m",
		"/api      (provider setup)",
		"/compact  ctrl+x c",
		"/clear",
		"/help",
		"/exit     ctrl+x q",
	}
}

func estimateContext(msgs []provider.Message) (tokens, pct int) {
	for _, m := range msgs {
		tokens += len(m.Content)/4 + 8
	}
	pct = tokens * 100 / 120000
	if pct > 99 {
		pct = 99
	}
	return tokens, pct
}

func commaify(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// wrapText breaks s into lines of at most n display cells (word-aware).
func wrapText(s string, n int) []string {
	if n < 4 {
		n = 4
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= n:
			cur += " " + w
		default:
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
