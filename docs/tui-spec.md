# OpenCode TUI — Implementation-Ready Spec (for a Go + Bubble Tea clone)

> Source of truth: official docs (`https://opencode.ai/docs/` — cli/tui, themes, cli/keybinds, commands)
> and the OpenCode source repo (`github.com/anomalyco/opencode`, branch `dev`, commit `cbe7159`),
> sparse-cloned at `packages/tui` and `packages/theme`.
> Every quoted value below comes from those sources. Anything marked **(inferred)** is a reasonable
> translation (usually TS/SolidJS → Go/Bubble Tea) that was not literally found in source.

---

## 1. Layout

The TUI is built with **OpenTUI** (`<box>` / `<text>` primitives) on SolidJS — *not* React/Ink.
Root component: `packages/tui/src/app.tsx` (root layout at lines 1404–1482).

### 1.1 Top-level structure (session route)

From `routes/session/index.tsx` (lines 1480–1672), outer to inner:

```
┌──────────────────────────────────────────────────────────┐
│ [session tabs strip]            (1 row, horizontal mode) │
├────────────┬─────────────────────────────────────────────┤
│            │  scrollbox (transcript, flex-grow)          │
│  sidebar   │    └─ "Jump to latest ↓" chip (when scrolled)│
│  (width 42)│                                             │
│  raised bg │─────────────────────────────────────────────│
│            │  composer + permission prompt stack (bottom)│
└────────────┴─────────────────────────────────────────────┘
```

- **Sidebar width: 42** (`SESSION_SIDEBAR_WIDTH`), background = `theme.background.raised.base`,
  contains a shimmer-animated title and the vertical session-tab rail (paddingTop 1).
- **Vertical tabs**: stride 3 rows per tab (2 in compact mode, breakpoint
  `SESSION_TABS_COMPACT_BREAKPOINT`), scrollable, drag-to-reorder, right-click context menu.
- **Horizontal tabs**: a single 1-row strip (`height={1}`) above the transcript; adaptive widths
  (`adaptiveSessionTabLayout`), overflow indicator `‹N` on the left, trailing `+ New session` slot.
  Spring-animated width changes (`spring({ visualDuration: 0.1 })`) — animations default **on**
  (`config.animations ?? true`).
- **Transcript**: a `<scrollbox>`; when not at the bottom, a floating **"Jump to latest ↓"** chip.
- **Bottom stack**: composer (editor) first, then the permission prompt when one is pending.

### 1.2 Dialog / overlay layering

From `ui/dialog.tsx`:

- Backdrop: full-screen box, `zIndex 3000`, `backgroundColor = RGBA.fromInts(0, 0, 0, 150)`
  (black at **alpha 150/255 ≈ 59%**), vertically positioned at `height / 4` unless `centered`.
- Panel: `width = dialogWidth(size)`, `maxWidth = terminalWidth - 2`,
  `backgroundColor = theme.background.base` (`@dialog.background.base = $background.raised.base`),
  `paddingTop 1`.
- Widths: **medium = 60, large = 88, xlarge = 116** columns.
- Insets: `DIALOG_INSET = 3`, `DIALOG_LIST_INSET = 4` (rows reserve 1 extra cell for the
  selection rail / marker).
- Clicking the backdrop closes; click-on-selection does not (copy-on-select guard).

### 1.3 Splash / startup

- `mini/splash.ts`: entry splash is a 2-row flex line — label `▪ oc mini` (mono: `[O] oc mini`),
  then leaf path, ` v{version}`, then the working directory, joined with ` · ` (mono: ` - `);
  stops adding segments when the line would overflow. Exit banner shows the cell-based `go` logo
  (mono: `[O]`) plus `Session  {title}` / `Continue opencode mini -s {session_id}` (bold).
- `component/startup-loading.tsx`: centered pill above the bottom line,
  `backgroundColor = raised.base`, `paddingLeft/Right 1`, muted spinner with text
  **"Loading plugins…"** → **"Finishing startup…"**. Appears only after **500 ms** of not-ready,
  hides **3000 ms** after ready (remaining time respected).
- Spinner frames (source): `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`.

---

## 2. Colors (hex table)

Default theme name: **`opencode`**; modes: `system` / `dark` / `light`.
Source: `packages/tui/src/theme/assets/v2/opencode.json`.

### 2.1 Semantic tokens (dark mode, resolved)

| Token | Hex | Notes |
|---|---|---|
| `background.base` | `#0a0a0a` | `$hue.neutral.800` |
| `background.raised.base` | `#141414` | `$hue.neutral.700` (sidebar, dialogs, prompt bg) |
| `background.raised.high` | `#1e1e1e` | `$hue.neutral.600` (toast, hover) |
| `background.raised.max` | `#4c4c4c` | `$hue.neutral.500` |
| `text.base` | `#eeeeee` | `$hue.neutral.200` |
| `text.muted` | `#808080` | `$hue.neutral.400` |
| `border.base` | `#484848` | literal |
| `scrollbar.base` | `#606060` | literal (also used as "borderActive" in docs) |
| `text.feedback.error.base` | `#e06c75` | `$hue.red.200` |
| `text.feedback.warning.base` | `#f5a742` | literal (v2) / `$hue.accent.200` in light |
| `text.feedback.success.base` | `#7fd88f` | `$hue.green.200` |
| `text.feedback.info.base` | `#56b6c2` | `$hue.cyan.200` |
| `hue.accent` (dark) | purple `#9d7cd8` (`200`) | headings, category headers |
| `hue.interactive` (dark) | orange `#fab283` (`200`) | links, focus, active markers |

### 2.2 Diff colors

| Token | Hex |
|---|---|
| `diff.text.added` | `#4fd6be` |
| `diff.text.removed` | `#c53b53` |
| `diff.text.context` / `hunkHeader` | `#828bb8` |
| `diff.background.added` | `#20303b` |
| `diff.background.removed` | `#37222c` |
| `diff.highlight.added` | `#b8db87` |
| `diff.highlight.removed` | `#e26a75` |
| `diff.lineNumber.text` | `#8f8f8f` |
| `diff.lineNumber.background.added` | `#1b2b34` |
| `diff.lineNumber.background.removed` | `#2d1f26` |

### 2.3 Syntax + markdown (dark)

| Token | Hex | Token | Hex |
|---|---|---|---|
| `syntax.comment` | `#808080` | `syntax.keyword` | `#9d7cd8` |
| `syntax.function` | `#fab283` | `syntax.variable` | `#e06c75` |
| `syntax.string` | `#7fd88f` | `syntax.number` | `#f5a742` |
| `syntax.type` | `#e5c07b` | `syntax.operator` | `#56b6c2` |
| `syntax.punctuation` | `#eeeeee` | | |
| `markdown.heading` | `#9d7cd8` | `markdown.link` | `#fab283` |
| `markdown.linkText` | `#56b6c2` | `markdown.code` | `#7fd88f` |
| `markdown.blockQuote` / `emphasis` | `#e5c07b` | `markdown.strong` | `#f5a742` |
| `markdown.listItem` | `#fab283` | `markdown.listEnumeration` | `#56b6c2` |

### 2.4 Light-mode deltas (exact overrides)

| Token | Light hex |
|---|---|
| `border.base` | `#b8b8b8` |
| `scrollbar.base` | `#a0a0a0` |
| `diff.text.added` | `#1e725c` |
| `diff.background.added` / `removed` | `#d5e5d5` / `#f7d8db` |
| `diff.highlight.added` / `removed` | `#4db380` / `#f52a65` |
| `diff.lineNumber.text` | `#595959` |
| `accent` / `interactive` | orange / blue |
| light gray ramp | `100 #000000 … 800 #ffffff` (inverted) |

Categorical palette (dark): `blue, purple, green, orange, red`; (light): `purple, orange, green, blue, red`.

---

## 3. Keybindings (table)

Source of truth: `packages/tui/src/config/v1/keybind.ts` (defaults), cross-checked against
`https://opencode.ai/docs/cli/keybinds`. **Leader = `ctrl+x`** (`LeaderDefault`).

### 3.1 Core / app

| Command | Default | Description |
|---|---|---|
| `leader` | `ctrl+x` | Leader for combos |
| `app.exit` | `ctrl+c`, `ctrl+d`, `leader q` | Exit application |
| `command_list` | `ctrl+p` | Command palette |
| `open_menu` | `ctrl+o` | Recent sessions & projects |
| `help_show` | `none` (docs: `ctrl+?` **(inferred)**) | Help dialog |
| `editor_open` | `leader e` | Open external editor |
| `theme_list` | `leader t` | List themes |
| `sidebar_toggle` | `leader b` | Toggle sidebar |
| `status_view` | `leader s` | View status |
| `terminal_suspend` | `ctrl+z` | Suspend terminal |

### 3.2 Sessions / tabs

| Command | Default |
|---|---|
| `session_new` | `leader n` |
| `session_list` | `leader l` |
| `session_compact` | `leader c` |
| `session_queued_prompts` | `leader q` |
| `session_export` | `leader x` |
| `session_timeline` | `leader g` |
| `session_rename` | `ctrl+r` |
| `session_delete` | `ctrl+d` |
| `session_interrupt` | `escape` |
| `session_background` | `ctrl+b` |
| `session_pin_toggle` | `ctrl+f` |
| `session_tab_next` / `previous` | `ctrl+tab` / `ctrl+shift+tab` (also `alt+down` / `alt+up`) |
| `session_tab_next_unread` / `previous` | `alt+shift+down` / `alt+shift+up` |
| `session_tab_close` | `leader w` |
| `session_tab_reopen` | `ctrl+shift+t` |
| `session_tab_select.1..10` | `leader 1..0`, `ctrl+1..0` |
| `session.first` / `session.last` | `ctrl+g,home,alt+home` / `ctrl+alt+g,end` |
| `session.page.up` / `down` | `pageup,ctrl+alt+b` / `pagedown,ctrl+alt+f` |
| `session.line.up` / `down` | `ctrl+alt+y` / `ctrl+alt+e` |
| `session.half.page.up` / `down` | `ctrl+alt+u` / `ctrl+alt+d` |
| `session.undo` / `session.redo` | `leader u` / `leader r` |
| `messages.copy` | `leader y` |

### 3.3 Model / agent

| Command | Default |
|---|---|
| `model_list` | `leader m` |
| `model.cycle_recent` / `_reverse` | `f2` / `shift+f2` |
| `model.dialog.favorite` | `ctrl+f` (in model dialog) |
| `model.dialog.provider` | `ctrl+a` (in model dialog) |
| `agent.cycle` | `shift+tab` |
| `agent.cycle.reverse` | `none` |
| `agent.list` | `leader a` |
| `variant.cycle` | `ctrl+t` |

### 3.4 Prompt / input editing

| Command | Default |
|---|---|
| `input.submit` | `return` |
| `input.newline` | `shift+return`, `ctrl+return`, `alt+return`, `ctrl+j` |
| `prompt.clear` | `ctrl+c` |
| `prompt.history.previous` / `next` | `up` / `down` (when input empty) |
| `input.line.home` / `end` | `ctrl+a` / `ctrl+e` |
| `input.delete.to.line.end` / `start` | `ctrl+k` / `ctrl+u` |
| `input.delete` | `ctrl+d`, `delete`, `shift+delete` |
| `input.backspace` | `backspace`, `shift+backspace` |
| `input.word.forward` / `backward` | `alt+f` / `alt+b` (+ `alt+right/left`, `ctrl+right/left`) |
| `input.delete.word.forward` / `backward` | `alt+d` / `ctrl+w` |
| `input.undo` / `redo` | `ctrl+-` / `ctrl+.` |
| `input.select_all` | `super+a` |
| `prompt.images.view` | `leader i` |
| `prompt.queue` | `leader return` |

### 3.5 Dialog / autocomplete (modal layer)

| Command | Default |
|---|---|
| `dialog.select.prev` / `next` | `up,ctrl+p` / `down,ctrl+n` |
| `dialog.select.page_up` / `page_down` | `pageup` / `pagedown` |
| `dialog.select.home` / `end` | `home` / `end` |
| `dialog.select.submit` | `return` |
| dialog cancel | `escape` |
| `prompt.autocomplete.prev` / `next` | `up,ctrl+p` / `down,ctrl+n` |
| `prompt.autocomplete.hide` | `escape` |
| `prompt.autocomplete.select` | `return` |
| `prompt.autocomplete.complete` | `tab` |
| `permission.prompt.fullscreen` | `ctrl+f` |
| section navigation (dialogs) | `alt+up` / `alt+down` |

### 3.6 Diff viewer

`enter`/`space` toggle, `right`/`left` expand/collapse, `E` expand all, `tab` switch focus,
`]`/`[` next/prev hunk, `n`/`p` next/prev file, `b` file tree, `s` single patch, `d` source,
`v` split/unified, `?` more shortcuts, `escape`/`q` close.

Docs confirm the user-facing set: leader combos (`ctrl+x m` model, `ctrl+x n` new session,
`ctrl+x l` sessions, `ctrl+x b` sidebar, `ctrl+x e` editor, `ctrl+x u/r` undo/redo), `ctrl+p`
palette, `ctrl+o` recent, `F2` cycle model, `Shift+Tab` cycle agent, `Enter` submit/steer,
`Shift/Ctrl/Alt+Enter` or `Ctrl+J` newline, `Esc` interrupt.

---

## 4. Messages / Transcript

Sources: `routes/session/index.tsx`, docs TUI page.

### 4.1 User message

- Left border: a single **`┃`** cell in the border color; content sits on a **raised background**
  (`background.raised.base`).
- Attachments render as inline chips: `` ` file `/path` `` and `` ` skill `name` `` (muted
  background, backtick-wrapped) — exact chip styling **(inferred)** from the rendered markup.
- Long text wraps (word wrap).

### 4.2 Assistant message

- Plain text on `background.base`, markdown-rendered per theme tokens (§2.3).
- **Assistant footer** row (after each assistant turn):
  `Build · model-name · 12.4s · interrupted`
  — mode name, model, elapsed duration, optional state; separators are ` · `, all muted
  except the model name **(inferred: model in `text.base`, rest `text.muted`)**.

### 4.3 Reasoning / thinking blocks

- Rendered as: `Thought: {title} · {duration}` with the text in the **warning** color
  (`#f5a742`), collapsed by default; toggleable via `session.toggle.thinking`
  (keybind default `none`).

### 4.4 Scrolling behavior

- Transcript is a scrollbox with acceleration (config `scrollAcceleration`).
- "Jump to latest ↓" chip appears when scrolled away from the bottom; `session.last`
  (`ctrl+alt+g`/`end`) jumps to the end.
- While streaming, auto-stick to bottom **(inferred)**.

---

## 5. Tool call blocks

Sources: `routes/session/index.tsx` (tool rendering), `util/*`.

### 5.1 Inline row (compact form)

- 2-cell status icon: **`✓`** on success, **`✗`** on error, spinner while running
  (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`), colored `success`/`error`/`muted` respectively **(inferred)**.
- Followed by the tool name and a one-line summary; appears inline in the transcript flow.

### 5.2 Block form (`BlockTool`)

- Bordered block with a **left bar** (1-cell vertical rule, border color) — same visual language
  as the `SplitBorder` (`┃` chars on left/right only).
- Header: tool name + state; body holds the tool-specific display.

### 5.3 Per-tool displays

| Tool | Rendering |
|---|---|
| **Shell / bash** | `$ {command}` header (command in `text.base`, `$` muted), output below; **collapsed to 10 lines** with an expand affordance when longer |
| **Edit** | Patch diff with label **`← Edit`** |
| **Write / create** | Label **`# Created`** |
| **ApplyPatch** | Label **`← Patched`** |
| **Read** | File path + content preview (line range if given) |
| **PatchDiff** | Unified diff using `diff.*` tokens (§2.2): added lines `#4fd6be` on `#20303b`, removed `#c53b53` on `#37222c`, line numbers `#8f8f8f` on `#1b2b34`/`#2d1f26`, hunk headers `#828bb8` |

- Diff viewer keys: §3.6. File tree toggle `b`, split/unified `v`, single patch `s`.

---

## 6. Editor (composer)

Sources: `component/prompt/index.tsx`, `feature-plugins/prompt/footer.tsx`.

- **Left border**: a single **`╹`** cell (vertical rule) — the composer's caret rail.
- **Background**: `decrease(background.raised.base)` → a dimmer variant of `#141414`
  (≈ `#0f0f0f`, **(inferred)** exact value depends on the `decrease` mix function).
- **Placeholder**: `Ask anything… "…"` — the second part cycles hints (dark mode default hints:
  `Fix a TODO in the codebase`, `What is the tech stack of this project?`,
  `Fix broken tests`; shell mode: `ls -la`, `git status`, `pwd`).
- **Max height**: `max(6, terminalHeight / 3)` rows, then internal scroll.
- **Metadata row** (inside composer, above footer): `agent · model provider · variant`,
  muted text with ` · ` separators.
- **Footer row** (below/attached): left group — context `12,345 (12%)` · cost `$0.42`;
  right group — key hints: `⇧tab agents`, `ctrl+p commands`;
  shell mode replaces hints with `esc exit shell mode`.
- Multi-line input via `shift/ctrl/alt+enter` or `ctrl+j`; submit `enter`; history `↑`/`↓`
  when the buffer is empty; `ctrl+c` clears.
- **External editor**: `ctrl+x e` (`editor_open`) — opens `$VISUAL`/`$EDITOR` **(inferred)**.

---

## 7. Status bar

- There is no single classic bottom status line; the persistent chrome is:
  1. **Session tabs strip** (top, 1 row horizontal / left rail vertical).
  2. **Composer footer** (context %, cost, key hints) — §6.
  3. **Home footer** (when no session): MCP server count · plugin count · version string
     (e.g. `3 MCP servers · 5 plugins · v1.x.x`) — exact grouping **(inferred)**.
  4. **Assistant footer** per turn — §4.2.
- Sidebar footer / session list rows show project name + git branch (`sessionTabDetail`:
  `projectName · branch`, default-branch detection, non-canonical directory marker).
- Key hints use lowercase `ctrl+`/`alt+` and glyph `⇧` for shift — from the footer source.

---

## 8. Overlays

### 8.1 Model picker (`Select model`)

- Dialog `DialogSelect`, size **large (88)** **(inferred)**, header `Select model`,
  search input placeholder **`Search models`** (or `searchInTitle` under new-designs).
- Options grouped by **provider** (category headers, accent-purple bold under old design,
  muted under new design), each row: model name, right-side footer (context window / cost)
  and favorite marker.
- Keys: `ctrl+f` favorite toggle, `ctrl+a` provider list, `f2`/`shift+f2` recent cycle,
  `↑/↓` or `ctrl+p/n` move, `pgup/pgdn` page, `home/end`, `enter` select, `esc` close.
- Current model marked with `▪` (new design) or `●` (old design) in the row gutter.

### 8.2 Session list (`Sessions`)

- Dialog, size **large/xlarge (88/116)**, title `Sessions`, search input below title.
- Rows: session title (truncated at **61** cols default), footer = relative time or project;
  pinned sessions first; category headers possible.
- Actions: `ctrl+f` pin/unpin, `ctrl+d` delete, `ctrl+r` rename, `enter` open.
- Selection rail: active row gets a 1-cell **`┃`** in `text.formfield.focused` at the left of
  the list (new design), full-row highlight `background.action.primary.focused` otherwise.
- `ctrl+o` opens the recent sessions/projects menu (same component, smaller size).

### 8.3 Command menu (`ctrl+p` / `Commands`)

- `DialogSelect` with `title="Commands"`, size **medium (60)** **(inferred)**.
- Rows = registered keymap commands with their bound key shown right-aligned (label resolved
  from `Keymap.all(command)`); fuzzy filter (fuzzysort, title weighted ×2 vs category/search text).
- Footer hints: primary group on the right, secondary on the left;
  empty state `No items available`, no-match state `No results found` (muted).

### 8.4 Approval prompt (permission)

Sources: `routes/session/permission.tsx`, `util/permission.ts`.

- Rendered **above the composer** in the bottom stack (not a centered dialog); full-screen
  variant via `ctrl+f`.
- Header: **`△ Permission required`** — triangle in the **warning** color `#f5a742`,
  text in `text.base` **(inferred)**.
- Body: tool name + command/description, optional diff preview.
- Options (vertical list, selected row highlighted):
  1. **`Allow once`**
  2. **`Always allow`**
  3. **`Reject`**
- Footer hints: `⇆ select` (left/right or up/down to move), `enter confirm`,
  `ctrl+f fullscreen`, `esc` reject **(inferred: esc maps to reject/cancel)**.

### 8.5 Toasts

Source: `ui/toast.tsx`.

- Absolutely positioned at **`top: 1, right: 2`**, `maxWidth = min(60, width - 6)`.
- Left/right borders only, using `SplitBorder` (`┃` chars), border color =
  `text.feedback[{variant}].base` (`info`/`success`/`warning`/`error`).
- Inner box: `background.raised.high` (`#1e1e1e`), padding 2 horizontal / 1 vertical.
- Optional **bold** title row + message; action affordance `› {label}` (muted; bold when
  hovered); no action → `x`. Queue shows `+N more` (muted).
- Default duration **5000 ms**; hover pauses; click activates action.

### 8.6 Autocomplete popup (slash / `@` mention)

Source: `component/prompt/autocomplete.tsx`.

- Anchored above the composer, popup list with a **`┃` selection rail** on the left.
- Rows: `display` padded to `maxDisplayLen + 2`, then description in muted text.
- Slash entries come from keymap commands (`/{name}`) + server commands (`/{name}`, MCP ones
  shown as `/{name}:mcp`), sorted alphabetically (`localeCompare`).
- Mention entries: `@file`, `@agent`, `@skill`, `@reference`.
- Keys: `↑/↓` or `ctrl+p/n`, `enter` select, `tab` complete, `esc` hide.

---

## 9. Slash commands

Registered via `slash: { name, aliases?, arguments? }` on keymap commands
(`app.tsx` lines 752–1200) plus server-defined commands (from `data.location.command.list`).
Built-ins observed in source:

| Slash | Aliases | Description |
|---|---|---|
| `/sessions` | `resume`, `continue` | (title: "Switch session") |
| `/new` | — | create a new session |
| `/clear` | — | Clear session |
| `/open` | `projects`, `project` | Open session or project |
| `/models` | — | List available models |
| `/agents` | — | List agents |
| `/usage` | — | View usage |
| `/mcps` | — | MCP servers |
| `/variants` | `thinking`, `effort` | Model variants |
| `/connect` | — | Connect an integration |
| `/highlights` | — | Show release highlights |
| `/settings` | — | Open settings |
| `/status` | — | View status |
| `/update` | — | Update OpenCode |
| `/pair` | `web` | Pair device |
| `/restart` | — | Restart service |
| `/reload` | — | Reload configuration |
| `/debug` | — | View debug info |
| `/themes` | — | Switch theme |
| `/help` | — | Help |
| `/exit` | `quit`, `q` | Exit the app |
| `/compact` | — | compact older session context to free space |
| `/editor` | — | compose in your external editor |
| `/undo` | — | Undo message (docs) |
| `/redo` | — | Redo message (docs) |

- Commands with `arguments: true` insert `/name ` and let the user type; others run immediately.
- Server/project commands are merged in, marked `queueable: true`.
- The popup lists them sorted alphabetically with the description right-padded per §8.6.

---

## 10. Misc

- **Logo**: ASCII/block logo in `logo.ts` / `component/logo.tsx`; the `go` mark uses `_` (full
  block bg), `^`/`~` (half-block `▀`) cells; mono fallback is `[O]`.
- **Sidebar**: width 42, `raised.base` background, shimmer title animation (inferred: a moving
  highlight across the title while a session is streaming).
- **Marquee**: overflowing tab titles scroll horizontally; edge fade applied to characters near
  the cut (`fadeTitleColor`), separators tinted 55% toward the background.
- **Tab status**: running tab → number in `hue.interactive[200]` (`#fab283`) with a pulse sweep;
  unread → glow toward `hue.accent[200]` (`#9d7cd8`); attention → feedback color; close `✕`
  appears on hover (held-hover close for mouse).
- **Scrollbars**: base `#606060` (dark) / `#a0a0a0` (light); hidden inside dialogs
  (`scrollbarOptions: { visible: false }`).
- **Animations**: default on; toggleable (`app.toggle.animations`); spinner + tab pulses +
  spring tab widths are the visible effects.
- **Selection/copy**: terminal-native selection, copy-on-select release guard in dialogs.
- **Theme system v2**: themes are JSON with `$hue.*` references, `base`/`light`/`dark` layers,
  `@surface` overrides (e.g. `@dialog`); installed themes live under `~/.config/opencode/themes`
  (docs) — clone should at least ship `opencode` dark+light.
- **Reduced/mono mode**: mono fallbacks exist for splash (`[O]`, ` - `, `...`) — useful when the
  terminal can't render block glyphs **(inferred: use in the Go clone as a fallback mode)**.

---

## 11. Clone checklist (Go + Bubble Tea mapping)

| Spec element | OpenCode source | Go / Bubble Tea component |
|---|---|---|
| Root layout, routing | `app.tsx` | `tea.Model` with a `layout` enum (home / session) + `lipgloss` row/column composition |
| Session route (scrollbox + composer stack) | `routes/session/index.tsx` | Custom `viewport.Model` for transcript + fixed-height bottom stack in `View()` |
| Sidebar (42 cols, raised bg) | `app.tsx` / session tabs | `lipgloss.Width(42)` column, `Background(#141414)`; hide on `ctrl+x b` |
| Horizontal tabs strip (1 row) | `component/session-tabs.tsx` | Single-line `view()` rendering adaptive widths; store order in model; marquee via `tea.Tick` |
| Vertical tab rail | same file | List widget with stride-3 rows, selection `┃` rail |
| Transcript messages | `routes/session/index.tsx` | `bubbletea/viewport` + `charmbracelet/lipgloss` + `goldmark` for markdown; user msg = `┃` left border + raised bg |
| Assistant footer | `AssistantFooter` | Rendered line: `mode · model · dur · state` |
| Tool inline row | `InlineToolRow` | 2-cell glyph (`✓`/`✗`/spinner) + text; spinner = `progress/spinner` with frames `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` |
| Tool block + left bar | `BlockTool` | `lipgloss.Border{Left: "┃"}` block, title row, collapsed 10-line output |
| Shell display | `ShellDisplay` | `$ cmd` header + output, collapse after 10 lines with `… more` |
| Patch diff | `PatchDiff` | Line renderer with `diff.*` colors (§2.2); labels `← Edit` / `# Created` / `← Patched` |
| Reasoning block | thought renderer | `Thought: title · dur` in `#f5a742`, collapsed |
| Composer/editor | `component/prompt/index.tsx` | Multi-line `textarea` (or custom buffer) with `╹` rail, bg `#0f0f0f`, max height `max(6, h/3)`, placeholder cycling hints |
| Composer footer | `prompt/footer.tsx` | Rendered row: `ctx (pct) · $cost` left, `⇧tab agents  ctrl+p commands` right |
| Permission prompt | `routes/session/permission.tsx` | Overlay component above composer: `△ Permission required` + 3 options + hints `⇆ select / enter confirm / ctrl+f fullscreen` |
| Dialog system | `ui/dialog.tsx` | Centered modal: widths 60/88/116, backdrop = translucent black (alpha 150), `esc` closes, `DIALOG_INSET=3` |
| DialogSelect lists | `ui/dialog-select.tsx` | Generic list modal: fuzzysort filter, `┃` rail, `▪/●` current marker, footer hint groups, page/home/end keys |
| Model picker | model dialog | `DialogSelect` "Select model", search `Search models`, provider categories, `ctrl+f`/`ctrl+a` |
| Session list | sessions dialog | `DialogSelect` "Sessions", size 88/116, pin/delete/rename actions |
| Command palette | `ctrl+p` command list | `DialogSelect` "Commands", fuzzy over command table + key labels |
| Autocomplete popup | `component/prompt/autocomplete.tsx` | Anchored popup above composer; `/` and `@` triggers; `enter`/`tab`/`esc` |
| Toasts | `ui/toast.tsx` | Top-right (`top 1, right 2`), width ≤ 60, `┃` side borders colored by variant, 5 s timer, queue `+N more` |
| Startup loading pill | `component/startup-loading.tsx` | Centered pill above bottom line; 500 ms show delay, 3000 ms hold |
| Splash | `mini/splash.ts` | Print entry/exit banner to scrollback (optional for clone) |
| Keybinds | `config/v1/keybind.ts` | `keymap` table in Go mirroring §3; leader `ctrl+x` chord handling (pending-chord state machine) |
| Theme tokens | `theme/assets/v2/opencode.json` | Go struct of hex values, dark/light tables (§2); `lipgloss.AdaptiveColor` or explicit mode switch |
| Slash commands | `app.tsx` + server commands | Command registry `{name, aliases, desc, run}` merged with server list; popup per §9 |
| Spinners/pulses | `component/spinner.tsx` etc. | `spinner.Spinner` (Braille) + `tea.Tick` at ~80 ms **(inferred: tick rate)** |
| Session tabs config | `config` (`tabs.indicators`, `animations`) | Config struct with `numbers|status` indicators, `animations bool` |

**Notes for the clone**

1. Bubble Tea has no native modal/backdrop layer — implement overlays as a `mode` field on the
   root model and render them last (they visually cover the transcript).
2. Translucent backdrops are impossible in terminals: use a solid `#0a0a0a`-dimmed fill or
   simply replace the full screen with the dialog surface **(inferred)**.
3. Alpha (`RGBA.fromInts(0,0,0,150)`) and `decrease(...)` color math must be pre-baked to flat
   hexes (e.g. dialog backdrop over `#0a0a0a` ≈ `#000000` at 59% → visually `#060606` **(inferred)**).
4. Keep all strings from §3/§9 byte-identical for a faithful clone; keep the exact hexes from §2.
