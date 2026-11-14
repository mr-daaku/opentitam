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
		w -= 43 // sidebar 42 + 1 gap
	}
	m.vp = viewport.New(maxInt(10, w), transcriptH)
	m.vp.Style = lipgloss.NewStyle().Padding(0, 1)
	m.ta.SetWidth(maxInt(20, m.width-6))
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

	// 2. Body row: optional sidebar + transcript.
	transcript := m.vp.View()
	body := transcript
	if m.sidebarOn && m.width > 80 {
		side := m.renderSidebar()
		body = lipgloss.JoinHorizontal(lipgloss.Top, side, transcript)
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

	// 6. Dialog overlays (rendered last → visually on top).
	switch m.overlay {
	case overlayModels, overlaySessions, overlayCommands:
		out = m.renderDialog(out)
	}
	return out
}

// renderSidebar is the 42-col session rail (spec §1.1).
func (m *Model) renderSidebar() string {
	var rows []string
	rows = append(rows, styleAccent.Render(" ⌬ OpenTitam"), "")
	items := sessionItems(m.store, m.sess.ID)
	for _, it := range items {
		line := truncate(it, 39)
		if strings.Contains(it, m.sess.ID) && m.sess.ID != "" {
			rows = append(rows, styleSideSel.Render("┃"+line))
		} else if it == "+ New session" {
			rows = append(rows, styleMuted.Render("  "+line))
		} else {
			rows = append(rows, styleMuted.Render("  "+line))
		}
	}
	rows = append(rows, "", styleMuted.Render("  v"+fmt.Sprint(0)+" · ctrl+x b"))
	content := strings.Join(rows, "\n")
	return styleSidebar.Height(m.height - 1).Render(content)
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

// renderDialog centers a DialogSelect-style panel (spec §8, widths 60/88).
func (m *Model) renderDialog(body string) string {
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

	// Center over body.
	bodyLines := strings.Split(body, "\n")
	panLines := strings.Split(panel, "\n")
	panW := lipgloss.Width(panel)
	row := (m.height - len(panLines)) / 2
	if row < 0 {
		row = 0
	}
	col := (m.width - panW) / 2
	if col < 0 {
		col = 0
	}
	for i, pl := range panLines {
		r := row + i
		for len(bodyLines) <= r {
			bodyLines = append(bodyLines, "")
		}
		line := bodyLines[r]
		left := clip(line, col)
		right := ""
		if idx := widthBy(line, col+panW); idx < len(line) {
			right = line[idx:]
		}
		bodyLines[r] = left + pl + right
	}
	return strings.Join(bodyLines, "\n")
}

// renderTranscript flattens blocks into viewport content (spec §4, §5).
func (m *Model) renderTranscript() {
	var b strings.Builder
	for _, blk := range m.blocks {
		switch blk.kind {
		case "user":
			// User message: ┃ left border + raised bg (spec §4.1).
			for i, line := range strings.Split(blk.body, "\n") {
				if i == 0 {
					b.WriteString(styleUserBody.Render("┃ "+line) + "\n")
				} else {
					b.WriteString(styleUserBody.Render("  "+line) + "\n")
				}
			}
			b.WriteString("\n")
		case "assistant":
			b.WriteString(indent(blk.body, "  ") + "\n\n")
		case "streaming":
			sp := spinFrames[m.spinFrame%len(spinFrames)]
			b.WriteString(styleMuted.Render("  "+sp+" ") + strings.ReplaceAll(indent(blk.body, "  "), "\n", "\n") + "\n\n")
		case "footer":
			b.WriteString(styleMuted.Render("  "+blk.body) + "\n\n")
		case "tool_start":
			icon := styleMuted.Render("⠋ ")
			if !blk.pending {
				icon = styleToolOK.Render("✓ ")
			}
			b.WriteString("  " + icon + styleToolLabel.Render(blk.label) + " " + styleMuted.Render(truncate(blk.body, 100)) + "\n\n")
		case "tool_end":
			icon := styleToolOK.Render("✓")
			if strings.HasPrefix(blk.body, "✗") {
				icon = styleToolErr.Render("✗")
			}
			lines := strings.SplitN(blk.body, "\n", 2)
			b.WriteString("  " + icon + " " + styleToolLabel.Render(blk.label) + "\n")
			if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
				b.WriteString(indent(lines[1], "    ") + "\n")
			}
			b.WriteString("\n")
		case "error":
			b.WriteString("  " + styleError.Render("✗ "+blk.label) + "\n" + indent(blk.body, "    ") + "\n\n")
		case "info":
			b.WriteString(styleMuted.Render("  · "+blk.label) + "\n" + indent(blk.body, "    ") + "\n\n")
		}
	}
	m.vp.SetContent(b.String())
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

func filterSlash(prefix string) []string {
	var out []string
	for _, c := range slashList() {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func slashList() []string {
	return []string{
		"/clear", "/compact", "/exit", "/help", "/models", "/new", "/sessions",
	}
}

func allCommands() []string {
	return []string{
		"/new      ctrl+x n",
		"/sessions ctrl+x l",
		"/models   ctrl+x m",
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

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// widthBy returns byte index where display width reaches w (approx: 1 cell/rune).
func widthBy(s string, w int) int {
	if w <= 0 {
		return 0
	}
	count := 0
	for i := range s {
		if count == w {
			return i
		}
		count++
	}
	return len(s)
}
