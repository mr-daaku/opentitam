# OpenCode TUI — Full Delta Spec (corrections + additions to `tui-spec.md`)

> **What this file is.** `docs/tui-spec.md` is the baseline and stays valid for colors (§2),
> layout/dialog geometry (§1.1–1.2), messages (§4), tool blocks (§5), permission prompt (§8.4),
> toasts (§8.5) and autocomplete (§8.6). This file supplies **only deltas and corrections** for the
> ten topics below, plus the final Go implementation checklist. Where this file and `tui-spec.md`
> disagree, **this file wins.**
>
> **Sources.** Repo `github.com/anomalyco/opencode` (the `sst/opencode` URL 301-redirects there),
> branch **`dev`**, sparse blob-less clone of `packages/tui`, `packages/core`, `packages/cli`,
> `packages/server`, `packages/util`. Official docs fetched live: `/docs/cli/tui`, `/docs/cli/keybinds`,
> `/docs/commands`, `/docs/providers`, `/docs/cli/`. Anything not literally present in those sources is
> marked **(inferred)**. Anything searched for and **not present** is marked **not found**.
>
> **Notation.** ` · ` is a single U+00B7 between two spaces. Glyphs are byte-exact from source.

---

## 1. Splash / logo

### 1.1 Corrections to `tui-spec.md`

| Where | `tui-spec.md` says | Correct value |
|---|---|---|
| §10 "Logo" | "ASCII/block logo in `logo.ts` / `component/logo.tsx`; the `go` mark uses `_` (full block bg), `^`/`~` (half-block `▀`) cells" | Incomplete — see §1.2 for the byte-exact art and §1.3 for the cell encoding (there are **four** marker chars, `_ ^ ~ ,`, and `,` is `▄`) |
| §1.3 Splash | entry splash = `mini/splash.ts` line with `▪ oc mini` | Correct, but scope is wrong: `entrySplash`/`exitSplash` are **`opencode mini` only** (`mini/splash.ts`, gated by `miniSettings.splash === "show"`). The full TUI has **no scrollback splash**; its "splash" is the home-screen `<Logo/>` + startup pill |
| §1.3 Splash | — | **Add** config key `mini.splash`: `Schema.Literals(["show","hide"])`, description *"Show or hide the entry and exit splash banners"*, default `"show"` (`config/index.tsx:224`, `mini/runtime.boot.ts:94`) |
| §11 checklist | "Splash \| `mini/splash.ts`" | Split into `Logo` (full TUI home) and `mini/splash.ts` (opencode mini only) |

### 1.2 Exact art — `packages/tui/src/logo.ts`

```ts
export const logo = {
  left:  ["                   ",
          "█▀▀█ █▀▀█ █▀▀█ █▀▀▄",
          "█__█ █__█ █^^^ █__█",
          "▀▀▀▀ █▀▀▀ ▀▀▀▀ ▀~~▀"],
  right: ["             ▄     ",
          "█▀▀▀ █▀▀█ █▀▀█ █▀▀█",
          "█___ █__█ █__█ █^^^",
          "▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀"],
}

export const go = {
  left:  ["    ", "█▀▀▀", "█_^█", "▀▀▀▀"],
  right: ["    ", "█▀▀█", "█__█", "▀▀▀▀"],
}
```

Both are **4 lines**; `logo.left` line 0 and `logo.right` line 0 are padding rows (leading spaces).

### 1.3 Cell encoding — `component/logo.tsx` `renderLine(line, fg, bold)`

`shadow = tint(theme.background.base, fg, 0.25)` (25 % mix of the glyph color into the page background).

| Char | Rendered as | fg | bg |
|---|---|---|---|
| `_` | space `" "` | `fg` | `shadow` |
| `^` | `▀` | `fg` | `shadow` |
| `~` | `▀` | `shadow` | (default) |
| `,` | `▄` | `shadow` | (default) |
| anything else (incl. `█`, `▀`, `▄`) | that char | `fg` | (default) |

`bold = true` → `TextAttributes.BOLD` on every cell. Every cell is `selectable={false}`.

### 1.4 Responsive tiers (exact predicate chain, `component/logo.tsx:54`)

```
height < 12                      → render nothing (null)
width  < 22                      → go.right.slice(1)      (3 rows, fg=text.base,   bold)
width  < 44                      → logo.left.slice(1)     (3 rows, fg=text.muted, not bold)
                                   then logo.right        (4 rows, fg=text.base,   bold)
otherwise (w ≥ 44, h ≥ 12)       → all 4 rows of logo.left, gap=1 cell, then logo.right[index]
                                   (left muted/not-bold, right base/bold)
```

Note the asymmetry: the narrow tier drops `logo.left[0]` but keeps all of `logo.right`.

### 1.5 Home route layout — `routes/home.tsx:81–108`

Outer column, `alignItems="center"`, `paddingLeft = paddingRight = (width < 44 ? 1 : 2)`:

```
<box flexGrow=1 minHeight=0/>        ← top spacer
<box height=3/>                      ← fixed 3-row gap
<Logo/>                              ← flexShrink=0; its measured width is kept as `logoWidth`
<box height=1/>
<UpdateNotification width={logoWidth}/>
<box width=100% maxWidth=75 paddingTop=1>  <Prompt/>   ← composer, capped at 75 cols
<box flexGrow=1 minHeight=0/>        ← bottom spacer
…
<box width=100%>  <Slot path="home.footer"/>  </box>
```

* A first-run **form prompt** overlays at `position:absolute, zIndex 2000, left 0, right 0, bottom 1,
  paddingLeft 2, paddingRight 2` when `forms()[0].id` exists.
* `--prompt` auto-submit waits for `local.model.ready && local.model.catalogReady && agent && model`
  and for the composer text to equal `args.prompt`, then calls `submit()` exactly once (`sent` flag).

### 1.6 Update notification (`routes/home.tsx:125–176`)

Centered under the logo, `width = logoWidth`, `maxWidth = "100%"`, `gap 1`, fade-in text in
`text.muted`. Content by state:

| State | Rendered |
|---|---|
| local, `type === "available"` | `<action.primary.selected>/update</action.primary.selected>` + ` to install v{version}` |
| local, `type === "installed"` | `…/exit` + ` restart to use v{version}` |
| remote (`source==="server" && remote`), `type === "available"` | `remote server update available` (no `/` prefix) |
| remote, other types | not rendered (`<Show when={!remote \|\| state.type === "available"}>`) |

Hover → background `theme.background.action.primary.hovered`. Click: remote → `update.dismiss()`,
installed → `exit()`, otherwise `update.open?.("notification")`.

### 1.7 Home footer — `feature-plugins/home/footer.tsx`

Gate for the whole row: **`height >= 14 && width >= 44`**. Row: `paddingTop/Bottom = (height < 16 ? 0 : 1)`,
`paddingLeft/Right 2`, `gap 2`, `flexShrink 0`, `<Slot path="home.footer.status"/>`, `flexGrow 1` spacer,
version pinned right.

| Segment | Shown when | Exact markup |
|---|---|---|
| MCP | `mcp.list().length > 0` | `⊙ ` glyph + text. If `failed > 0`: glyph in `feedback.error`, text `{failed} MCP failed`. Else glyph in `feedback.success` when `count > 0` else `text.muted`, text `{count} MCP`. Muted `/mcps` suffix **only if `width >= 64`**. Click → `keymap.dispatch("mcp.list")` |
| Plugins | `failed > 0` | `⊙ ` in `feedback.error` + `{failed} plugin{s} failed` (plural `s` when `!= 1`). Muted `/plugins` suffix **only if `width >= 80`**. Click → `keymap.dispatch("plugins.list")` |
| Version | `width >= 64` | `text.app.version` in `text.muted` |

`homeFooterVisibility(width)` → `{ mcpCommand: width >= 64, pluginCommand: width >= 80, version: width >= 64 }`.
This is **not** "3 MCP servers · 5 plugins · v1.x.x" as guessed in `tui-spec.md` §7 — the segments are
independent, unpadded, `gap 2`, and there is no literal "servers"/"plugins" wording.

### 1.8 Startup loading pill — `component/startup-loading.tsx`

`position:absolute, zIndex 5000, left 0, right 0, bottom 1, justifyContent center, alignItems center`.
Text: **`Loading plugins…`** while not ready → **`Finishing startup…`** when ready.
Show delay **500 ms**; after ready, hide at **3000 ms** (`const left = 3000 - (Date.now() - stamp)`).
Spinner = `component/spinner.ts` default frames `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`.

---

## 2. Slash commands

### 2.1 Corrections to `tui-spec.md` §9

* The table is **stale and incomplete**. Registered slash names are scattered across `app.tsx`,
  `routes/session/index.tsx`, `component/prompt/index.tsx`, `component/session-frame.tsx`,
  `feature-plugins/{prompt,system}/*.tsx` — `tui-spec.md` merged only `app.tsx` plus a few.
* **Missing entirely**: `/cd /editor /skills /worktrees /terminal /btw /diff /plugins /stats /rename
  /timeline /fork /compact /copy /export /undo /redo /terminal`.
* **Wrong/absent descriptions** — every row below carries the literal `title` from source.
* **not found** (searched all of `packages/tui` and `packages/core`, zero hits):
  **`/share`, `/cost`, `/retry`, `/switch`, `/step`, `/branch`, `/auth`, `/ttft`, `/mcp`**
  (the MCP command is **`/mcps`**, not `/mcp`). Do not implement them.
* `/themes` is plural (`slash: { name: "themes" }`); the *palette* command id is `theme.switch`.

### 2.2 Complete client-side list (38), with source file and aliases

| # | Slash | Aliases | `title` (exact) | Registered in |
|---|---|---|---|---|
| 1 | `/sessions` | `resume`, `continue` | `Switch session` | `app.tsx` |
| 2 | `/new` | — | `New session` | `app.tsx` |
| 3 | `/clear` | — | `Clear session` | `app.tsx` |
| 4 | `/open` | `projects`, `project` | `Open session or project` | `app.tsx` |
| 5 | `/models` | — | `Switch model` | `app.tsx` |
| 6 | `/agents` | — | `Switch agent` | `app.tsx` |
| 7 | `/usage` | — | `View usage` | `app.tsx` |
| 8 | `/mcps` | — | `MCP servers` | `app.tsx` |
| 9 | `/variants` | `thinking`, `effort` | `Switch model variant` | `app.tsx` |
| 10 | `/connect` | — | `Connect an integration` | `app.tsx` |
| 11 | `/highlights` | — | `Show release highlights` | `app.tsx` (conditional) |
| 12 | `/settings` | — | `Open settings` | `app.tsx` |
| 13 | `/status` | — | `View status` | `app.tsx` |
| 14 | `/update` | — | `Update OpenCode` | `app.tsx` (conditional) |
| 15 | `/pair` | `web` | `Pair device` | `app.tsx` |
| 16 | `/restart` | — | `Restart service` | `app.tsx` (conditional) |
| 17 | `/reload` | — | `Reload configuration` | `app.tsx` |
| 18 | `/debug` | — | `View debug info` | `app.tsx` |
| 19 | `/themes` | — | `Switch theme` | `app.tsx` |
| 20 | `/help` | — | `Help` | `app.tsx` |
| 21 | `/exit` | `quit`, `q` | `Exit the app` | `app.tsx` |
| 22 | `/terminal` | — | `New terminal` (id `session.terminal`) | `component/session-frame.tsx` |
| 23 | `/rename` | — | `Rename session` (id `session.rename`) **`arguments: true`** | `routes/session/index.tsx` |
| 24 | `/timeline` | — | `Jump to message` (id `session.timeline`) | `routes/session/index.tsx` |
| 25 | `/fork` | — | `Fork session` (id `session.fork`) | `routes/session/index.tsx` |
| 26 | `/compact` | — | `Compact session` (id `session.compact`) | `routes/session/index.tsx` |
| 27 | `/undo` | — | `Undo previous message` (id `session.undo`) | `routes/session/index.tsx` |
| 28 | `/redo` | — | `Redo` (id `session.redo`) | `routes/session/index.tsx` |
| 29 | `/copy` | — | `Copy last assistant message` (id `messages.copy`) | `routes/session/index.tsx` |
| 30 | `/export` | — | `Export session transcript` (id `session.export`) | `routes/session/index.tsx` |
| 31 | `/cd` | — | `Change working directory` (id `session.cd`) **`arguments: true`** | `component/prompt/index.tsx` |
| 32 | `/editor` | — | `Open editor` | `component/prompt/index.tsx` |
| 33 | `/skills` | — | `Skills` | `component/prompt/index.tsx` |
| 34 | `/worktrees` | — | `Manage workspaces` | `component/prompt/index.tsx` |
| 35 | `/btw` | — | `Ask a side question` (id `session.aside`) **`arguments: true`** | `feature-plugins/prompt/btw.tsx` |
| 36 | `/plugins` | — | `Plugins` (id `plugins.list`) | `feature-plugins/system/plugins.tsx` |
| 37 | `/stats` | — | `Usage statistics` (id `stats.open`) | `feature-plugins/system/stats.tsx` |
| 38 | `/diff` | — | `Open diff viewer` (id `diff.open`) | `feature-plugins/system/diff-viewer.tsx` |

Registration shape is `slash: { name, aliases?, arguments? }` on a keymap command.
**`arguments: true`** → the TUI inserts `/name ` and leaves the caret for typing; otherwise the command
runs immediately on selection.

### 2.3 Server / project commands

From `packages/core/src/plugin/command.ts`:

| Slash | `description` (exact) |
|---|---|
| `/init` | `guided AGENTS.md setup` |
| `/review` | `review changes [commit\|branch\|pr], defaults to uncommitted` |

Plus any **MCP server prompt** (displayed as `/{name}` with the server id) and any **user command**:

* Global: `~/.config/opencode/commands/**.md` (legacy singular dir `command/` also discovered).
* Project: `.opencode/commands/**.md`.
* **Only `.md` files** are discovered; nested paths become `/`-separated names
  (`.opencode/commands/team/review.md` → `/team/review`).
* Frontmatter fields: `description`, `agent`, `model`, `subagent` (alias `subtask`, deprecated;
  `subagent` wins if both present). **No `template` in Markdown** — the trimmed body is the template.
* JSON/JSONC `commands.<name>` requires `template`, accepts `description`, `agent`, `model`,
  `subagent`, `subtask`.
* `$ARGUMENTS` = full argument string; `$1`/`$2…` = parsed positions (quotes group and are stripped;
  the highest-numbered placeholder swallows the rest; missing → empty string). With no placeholder,
  non-empty arguments are appended after a blank line.
* `` !`cmd` `` blocks run the shell and splice stdout into the prompt **before** submission.
* Load order: project overrides global, nearer overrides ancestor, later replaces earlier;
  a custom command can replace a built-in. Files hot-reload (no restart).

### 2.4 Autocomplete popup for `/` (delta to §8.6)

Entries = keymap slash commands (`/{name}` + aliases) ∪ server commands ∪ MCP prompts, sorted with
`localeCompare`. Row = `display` padded to `maxDisplayLen + 2`, then description in `text.muted`,
`┃` selection rail on the left. Keys `↑/↓` or `ctrl+p/n`, `enter` select, `tab` complete, `esc` hide.

---

## 3. Command palette

Source: `component/command-palette.tsx` (74 lines), `ui/dialog-select.tsx`.

### 3.1 Corrections to `tui-spec.md` §8.3

| `tui-spec.md` §8.3 | Actual |
|---|---|
| "size **medium (60)** (inferred)" | Size is whatever `DialogSelect` defaults to **(inferred)** — `CommandPaletteDialog` passes no `size` prop |
| "fuzzy filter (fuzzysort, title weighted ×2 vs category/search text)" | `filterThreshold={0.7}` is explicit; the weighting is **(inferred)** |
| "Rows = registered keymap commands with their bound key shown right-aligned" | Correct, plus: rows also include **settings entries**, and the unfiltered list is prefixed with a **`Suggested`** category |
| — | **Add** `title="Commands"`, `searchInTitle`, `titlePlaceholder="Search commands"`, `flat={true}` |
| — | **Add** footer = the command's keybind label (`footer = shortcuts.all(command.id)`), and `searchFooter = [group, footer].join(" · ")` |

### 3.2 Exact construction

```tsx
<DialogSelect title="Commands" searchInTitle titlePlaceholder="Search commands"
              options={list()} flat filterThreshold={0.7} />
```

Option object per keymap command (skipped when `!command.id || !command.palette ||
command.id === COMMAND_PALETTE_COMMAND`):

```
title:       command.title ?? command.id
description: command.description
category:    command.group
searchText:  [command.id, command.description].filter(Boolean).join(" ")
searchFooter:[command.group, footer].filter(Boolean).join(" · ")
footer:      shortcuts.all(command.id)          // right-aligned key label
value:       command.id
suggested:   command.suggested (bool or () => bool)
onSelect:    dialog.clear(); command.run()
```

Settings options (from `settings` in `dialog-config.tsx`):

```
title:       setting.title
category:    setting.category
searchText:  setting.keywords?.join(" ")
searchFooter:`Settings · ${setting.category}`
value:       `setting:${settingID(setting)}`
onSelect:    dialog.replace(() => <DialogConfig current={settingID(setting)} />)
```

`list()`:

* **While a filter is active** (`ref.filter` truthy): `[...options(), ...settingOptions]`.
* **Otherwise**: `[...options().filter(suggested).map(o => ({...o, value: "suggested:"+o.value, category:"Suggested"})), ...options()]`
  — i.e. suggested commands appear **twice**, once under `Suggested`.

`searchInTitle` only takes effect under the `newDesigns()` experiment (`ui/dialog-select.tsx:655`);
otherwise the placeholder falls back to `props.placeholder ?? "Search"`.

---

## 4. Auth / provider UX

### 4.1 CLI surface — `packages/cli/src/commands/commands.ts`

`opencode auth` → description **`manage integrations and credentials`** (global flag group `ServerFlags`):

| Subcommand | Description (exact) | Arguments / flags |
|---|---|---|
| `auth list` | `list integrations and credentials` | `--format default\|json` (default `default`) |
| `auth login` | `connect an integration` | `[target]` = "Integration ID, name, or well-known provider URL"; `--method <id>`; `--answer key=value` (repeatable, at most 100) |
| `auth logout` | `log out of a saved account` | `[target]` = "Integration ID or name"; `[credential]` = "Credential ID or label (opens an account picker when omitted)" |
| `auth export` | `print stored credentials, including secrets, as JSON` | `[target]` (omitted → every integration) |
| `auth import` | `import credentials exported by auth export` | `[file]` (omitted → stdin) |
| `auth switch` | `switch the active account for an integration` | `[target]`, `[credential]` |

Related CLI entries (exact descriptions): `opencode models` = `List all available models`;
`opencode stats` = `Show shareable usage statistics`; `opencode mcp` = `Manage MCP (Model Context Protocol) servers`;
`opencode plugins` = `Manage plugins`; `opencode sessions` = `Manage sessions`;
`opencode service` = `Manage the background server` (`start|restart|status|stop|get|set|unset`);
`opencode reload` = `Reload configuration`; `opencode pair` = `Print one-time links to connect a browser or app`;
`opencode run` = `Run OpenCode with a message`; `opencode mini` = `Start the minimal interactive interface`;
`opencode debug` = `Debugging and troubleshooting tools`; `opencode acp` = `Start an Agent Client Protocol server`;
`opencode serve` = `Start the v2 API and web server`; `opencode update` = `Upgrade OpenCode to the latest or a specific version`;
`opencode uninstall` = `Uninstall OpenCode, keeping session data, configuration, and state`.

### 4.2 TUI `/connect` → `component/dialog-integration.tsx`

Dialog titles, in order of appearance:

| Stage | Title (exact) |
|---|---|
| integration picker | `Connect an integration` |
| method picker | `Connect {integration.name}` |
| single method | skipped (`if (methods.length === 1) return openMethod(...)`) |
| connected accounts view | `{integration.name}` (category header `Connected accounts`, action row `add`) |
| rename | `Rename account` |

Method kinds (`ConnectMethod = Exclude<IntegrationInfo["methods"][number], {type:"env"}>`):

| `type` | UI | Key strings |
|---|---|---|
| `key` | prompt dialog | title `method.label ?? "Connect {integration.name}"`, **`placeholder="API key"`**, calls `integration.connect.key`; failure renders an **inline error inside the dialog** (no toast) |
| `oauth` (code / auto) | `OAuthView` | messages `Starting authorization…` → `Connecting…` → `Opening link…`; link row uses `dialog.integration.copy` (`tab` = `Copy authorization link`) |
| `command` | `CommandView` | messages `Starting command…` → `Waiting for command to finish…`; shows streamed `output` |
| `external` | external-browser handoff | `dialog.integration.copy` / `dialog.integration.rename` (`ctrl+r`) / `dialog.integration.delete` (`ctrl+d`) |

Success → `toast.show({ variant: "success", message: "Connected {integration.name}" })`, then
`onConnected(providerID(data, location, integration.id))`. `env`-type methods are filtered out of the picker.

### 4.3 First-run / onboarding

* **Free-models dialog** — `component/dialog-model-free.tsx`:
  `title="Free models"`, `description="Connect a provider for access to more models"`.
* **`local.tsx:402`** fallback option label: `Connect a provider`.
* **Sidebar onboarding card** — `feature-plugins/sidebar/footer.tsx`, id `sidebar.footer.getting-started`:
  * Visible only when **`height >= 22`**, storage flag `getting-started.dismissed` is false, and
    `!hasConnectedProvider(list)`.
  * Background **`background.raised.high` (`#1e1e1e`)**, `paddingTop/Bottom 1`, `paddingLeft/Right 2`, `gap 1`.
  * Left glyph **`⬖`** in `text.base`.
  * Header row: **`Getting started`** (bold, `text.base`) … `✕` (muted) right-aligned; click sets `dismissed = true`.
  * Body line 1: `OpenCode includes free models so you can start immediately.`
  * Body line 2: `Connect from 75+ providers to use other models, including Claude, GPT, Gemini etc`
  * Action row: `Connect provider` (base) … `/connect` (muted), click → `keymap.dispatch("provider.connect")`.
* Legacy `auth.json` is imported into the SQLite store on upgrade **(inferred: same migration as
  `opencode auth import`; the TUI itself only reads the store)**.

---

## 5. Images

### 5.1 Clipboard read — `clipboard.ts`

```ts
clipboard.read({ preferredTypes: ["image/png", "text/plain"], selection: "clipboard" })
```

Statuses: `failed` → throw `result.error`; `timed-out` → `Error("Clipboard read timed out")`;
`unsupported` → `Error("Clipboard read is not supported on this system")`;
**`limit-exceeded` → `RangeError("Clipboard content exceeded configured read or image conversion limits")`**;
anything else non-`read` → `undefined`.

* `image/png` → `{ data: base64(bytes), mime: "image/png" }`.
* `text/plain` with **0 bytes → `undefined`** (this is the Windows image-only-paste path: it produces an
  empty bracketed paste, which the textarea reports as a paste and the TUI dispatches as `prompt.paste`).
* any other MIME → `Error("Unexpected clipboard MIME type: {mime}")`.
* `write()` strips NUL (`replaceAll("\0","")`), `destination: "all-available"`, `selection: "clipboard"`.
  Success = `host.status === "written" || terminal.status === "attempted"`, else
  `Error("Clipboard write failed (host: …, terminal: …)")`.

Storage: attachments carry `data:<mime>;base64` URIs; the paste command builds the data URL.
Local-file attachments accept `image/*` and `application/pdf` (`local-attachment.ts`).

### 5.2 Composer preview row — `component/prompt/index.tsx:1698–1764`

* Gated by **`config.prompt.image_preview`** *and* `visibleImageAttachments().length > 0`.
* Row height = `imagePreviewHeight() + 1` (the +1 is `paddingBottom 1`), `gap 1`, `justifyContent flex-start`.
* ```
  imagePreviewHeight = max(4, min(8, floor(terminalHeight / 4)))
  imagePreviewWidth  = imagePreviewHeight * 2
  visibleImageAttachments = imageAttachments().slice(0, 3)
  ```
* Each tile: `width = flexBasis = imagePreviewWidth`, `height = imagePreviewHeight`, `<image fit="cover"
  protocol="auto" width=100% height=100%/>`. On error → centered muted **`No preview`**.
  Left-click a tile → `openImagePreview(index)`.
* Overflow tile: `width 8`, height `imagePreviewHeight`, muted truncated
  **`+{total - visible} more`**; click → `openImagePreview(visibleImageAttachments.length)`.
* The same formula is reused for transcript/tool images in `routes/session/index.tsx`
  (`SessionImages`): `height = max(4, min(8, floor(h/4)))`, `visible = slice(0,3)`, row
  `paddingTop 1, paddingLeft (props.paddingLeft ?? 3), paddingRight 2, paddingBottom 1, gap 1`,
  gated by **`config.session.image_preview`** (note: *different* config key than the composer's
  `prompt.image_preview`).

### 5.3 Viewer dialog — `component/dialog-image-preview.tsx`

* Header: **`Image {index+1} of {images.length}`** (`DialogHeader`).
* Keys: `left` = `Previous image`, `right` = `Next image` (group `Dialog`).
* Footer hint chips: title `previous` key `←`, title `next` key `→`.
* Body caption: `current().mention?.text ?? "Image {index+1}"`; on failure → **`No preview`**.
* Opened from `prompt.images.view` (`leader i`), from a composer tile, from a transcript image, and from
  the diff viewer (`feature-plugins/system/diff-viewer-image.tsx`).

### 5.4 Message-side chips

File/skill/agent mentions inside the composer render as extmark-styled chips (`prompt-part` extmarks,
`extmarkToPart` map, styles `file` / `skill` / `agent` from `component/prompt/autocomplete.tsx:199`).
Transcript-side, a user message renders `` ` file `/path` `` / `` ` skill `name` `` chips —
exact chip colors remain **(inferred)** per `tui-spec.md` §4.1.

---

## 6. Tabs + sidebar

### 6.1 Constants — `context/session-tabs-model.ts` and `component/session-tabs.tsx`

| Constant | Value |
|---|---|
| `NEW_SESSION_TAB_TITLE` | `New session` |
| `SESSION_TAB_WIDTH` | **22** |
| `SESSION_TAB_MAX_WIDTH` | **32** |
| `SESSION_TAB_MIN_WIDTH` | **8** |
| `sessionTabOverflowWidth(count)` | `String(count).length + 2` |
| `FADE_WIDTH` | **4** |
| `ADD_TAB_WIDTH` | **3** |
| `MARQUEE_DELAY` | **600** ms |
| `MARQUEE_INTERVAL` | **80** ms |

Adaptive width: `roomy = contentWidth >= SESSION_TAB_WIDTH * visible.length`.
If roomy → `total = min(contentWidth, SESSION_TAB_MAX_WIDTH * visible.length)` and distribute the slack
evenly across `visible.length - 1` gaps, floored at `SESSION_TAB_MIN_WIDTH`.
Tab count from width: `roomy ? floor(width / MIN) : 1 + floor((width - SESSION_TAB_WIDTH) / MIN)`.
Horizontal content width subtracts `ADD_TAB_WIDTH` when the `+` slot is shown.

### 6.2 Overflow indicators (exact glyphs)

* Left: **`‹{before}`** (`session-tabs.tsx:1546`).
* Right: **`{after} ›`** — rendered as `{" " + layout().after}›` (`session-tabs.tsx:1752`), i.e. a
  space then the count then `›`.
* Width per indicator = `len + 2` (from `sessionTabOverflowWidth`).

### 6.3 Marquee / fade

* Start delay **600 ms**, then step every **80 ms**; only if `marqueeOverflows(title, width)`.
  Cycle width = `marqueeCycleWidth(title)`; `marqueeTextParts(value, width, offset)` slices the window.
* Fade ramp: `fade(position) = position <= 0 ? 0 : 0.2 + 0.72 * ((position - 1) / (FADE_WIDTH - 1))`
  → first visible char at **0.20**, last at **0.92**, stepping **0.24** per cell, over **4** cells.
  `start = index < FADE_WIDTH ? FADE_WIDTH - index : 0`, `end = index - (length - FADE_WIDTH) + 1`.
* Marquee is per-tab: `enter(sessionID, title, width)`, `leave(sessionID)`, `leaveHovered`, `reset`;
  a tab whose `sessionID` disappears from `items()` forces `reset()`.

### 6.4 `TabIndicator` — status vs numbers

Label priority (exact order, `session-tabs.tsx:172–178`):

```
if (numbers)                       → props.label              // sessionTabNumberLabel(index)
if (attention === "permission")    → "!"
if (attention === "question")      → "?"
if (runs())                        → TAB_SPINNERS[spinner].frames[0]   // default spinner "dots"
if (props.label === "+")           → "+"
if (unread || fading())            → TAB_UNREAD_MARKERS[unreadMarker]   // default "small-dot"
```

* `runs = status.busy && !status.attention`; `pending = busy || attention`.
* Unread color: `hue.accent[200]` (`#9d7cd8`) normally; `tabFeedbackColor()` returns
  `hue.accent[200]` when `status.attention`, `feedback.error.base` (`#e06c75`) when `unread === "error"`.
* Glowing/flash: `tint(backgroundColor, tint(unreadColor, flashColor, flash * 0.3), min(1, opacity / 0.8))`
  and unread tint `tint(base, glow, 0.12 * unreadGlowIntensity(index, width) * level)`.
* Config: `config.tabs.indicators: "status" | "numbers"`; also `spinner` and `unreadMarker` overrides
  (storybook exposes both option sets).
* Active tab: bold + lifted background; running number in `hue.interactive[200]` (`#fab283`);
  hover → `✕` at `right: 1` (`session-tabs.tsx:1097` vertical, `:1743` horizontal); middle-click closes;
  right-click opens the context menu; drag reorders; spring `visualDuration 0.1`.
* Vertical rail stride: **3** rows normally, **2** in compact mode; tab block height `compact ? 3 :
  (newTab ? 1 : 2)`; `paddingY = compact ? 1 : 0`.
* Horizontal strip is a single `height={1}` row; the `+` slot is `width={ADD_TAB_WIDTH}` (3).

### 6.5 Sidebar

* Width **42** (`SESSION_SIDEBAR_WIDTH`), background `theme.background.raised.base`.
* Slot chain: `sidebar.content` (top) and `sidebar.footer` (bottom).
* **`sidebar.content` — Context block** (`feature-plugins/sidebar/context.tsx`):
  shown only when `state || cost > 0`; header **`Context`** in `text.base` (bold);
  then `{tokens.toLocaleString()} tokens`; then `{percent}% used` when `percent !== undefined`;
  then `{money.format(cost)} spent` when `cost > 0`. `money = Intl.NumberFormat("en-US", {style:"currency", currency:"USD"})`.
  `tokens = input + output + reasoning + cache.read + cache.write` of the last assistant message with usage;
  `percent = round(tokens / model.limit.context * 100)` (undefined when the model has no context limit).
* **`Limit` block** (`sidebar/limit.tsx`): header **`Limit`** bold; value **`{formatDollars(left)} remaining`**,
  in `feedback.error.base` when `usageLevel ∈ {out, reached}` else `text.muted`. Hidden when the selected
  model has no applicable limit.
* **`sidebar.footer`**: the §4.3 onboarding card, then a `FilePath` of
  `{formattedDir}:{branch}` (branch omitted when none) with **`maxWidth={38}`**; hovered → `text.base`, else `text.muted`.

---

## 7. Composer

Sources: `component/prompt/index.tsx`, `component/prompt/metadata.tsx`, `feature-plugins/prompt/footer.tsx`.

### 7.1 Corrections to `tui-spec.md` §6

| `tui-spec.md` §6 | Actual |
|---|---|
| "Left border: a single `╹` cell" | Correct, and it is `customBorderChars.vertical = "╹"` **only when `promptBg().a !== 0`**, else a space. The border color is `tint(theme.border.base, highlight(), agentMetaAlpha())` — it tints toward the agent color while metadata fades in |
| — | **Add** a 1-row **top shade strip**: `border=["bottom"]`, `customBorderChars.horizontal = "▀"`, `borderColor = promptBg` (space when alpha is 0) |
| "Background: `decrease(background.raised.base)` … ≈ `#0f0f0f`" | Exact expression: `theme.decrease(theme.background.raised.base)` — keep the function, don't hard-code **(inferred: pre-bake to `#0f0f0f`)** |
| "Placeholder … second part cycles hints" | Exact truncation: `width = (termWidth < 44 ? termWidth - 5 : min(75, termWidth - 4) - 5)`, then `Locale.takeWidth(value, max(1, width)).trimEnd()`. Index advances on submit/blur (`randomIndex(list.length)`), not on a timer |
| "Metadata row: `agent · model provider · variant`" | See §7.3 — plus `auto`, a `limit` field, an explicit drop order, and different colors |
| "Footer row: left `12,345 (12%)` · `$0.42`; right `⇧tab agents`, `ctrl+p commands`" | See §7.4 — `⇧tab agents` is **only** shown when there is no usage status, no goal and no running subagent/shell (and `width >= 44`); `ctrl+p commands` shows whenever the layout budget allows |
| — | **Add** running state: blocks spinner @ 40 ms + `esc interrupt` / `esc again to interrupt` (double-Esc within 5 s) |

### 7.2 Geometry

```
maxHeight          = max(6, floor(terminalHeight / 3))
promptBg           = theme.decrease(theme.background.raised.base)
inner paddingLeft  = paddingRight = (width < 44 ? 1 : 2), paddingTop 1
cursorColor        = disabled ? background.raised.base : text.base
focusedBackgroundColor = "transparent"
```

### 7.3 Metadata row — `component/prompt/metadata.tsx`

Layout budget: **`width = max(0, terminalWidth - (terminalWidth < 44 ? 9 : 13))`**.

Shell mode short-circuits to `{ agent: props.agent ?? "Shell", model: "" }`.

Candidate layouts, first that fits `stringWidth(text(candidate)) <= width` wins:

1. `{agent, auto, model, provider, variant, limit}`
2. `{agent, model, provider, variant, limit}`            ← drop `auto`
3. `{agent, model, provider: provider.split(" / ").at(-1), variant, limit}` ← shorten provider
4. `{agent, model, variant, limit}`                       ← drop provider
5. fallback: `{agent, model: truncateWidth(model, max(9, width - len(prefix) - len(suffix))), variant, limit}`
   where `prefix = agent ? agent + " · " : ""`, `suffix = (variant ? " · " + variant : "") + (limit ? "  " + limit : "")`,
   and trailing whitespace before `…` is collapsed (`/\s+…$/ → "…"`).

Visibility rules inside `promptMetadataLayout`: `agent` is dropped when `terminalWidth < 44`;
`provider` becomes `""` when `terminalWidth < 44`.

Rendered tokens (row `gap 1`):

| Token | Color |
|---|---|
| `agent` (titlecased) | `fade(highlight, agentAlpha)` (agent color) |
| `auto` (only when `local.permission.mode === "autoaccept"` and mode is normal) | `text.muted` |
| `·` separators | `text.muted` |
| `model` | `text.muted` when `muted`, else `text.base` |
| `provider` | `text.muted` |
| `variant` | **`text.feedback.warning.base` + BOLD** |
| `limit` (1 leading space) | `text.muted`, or `text.feedback.error.base` when `danger` |

All alpha-faded via `fade(color, alpha) = RGBA.fromValues(r,g,b, a*alpha)`; alphas animate independently
(`agentMetaAlpha`, `modelMetaAlpha`, `variantMetaAlpha`) using `createFadeIn`.
The whole metadata block is hidden (`<box height={1}/>`) when neither `agent` nor `mode==="shell"` is set.

### 7.4 Footer row — `feature-plugins/prompt/footer.tsx`

The composer's last row is `<box flexDirection="row" justifyContent="space-between" gap={2}>` containing
`<Slot path="prompt.footer">` → `<Slot path="prompt.footer.status">`.

**Left group**, in order, joined by ` · `, all `text.muted` unless noted:

```
[{shortcut("session.child.first")} ]{N subagent(s)}[ · ][{N shell(s)}]
[ · ][GoalStatus…]                       ← only when a goal exists
[ · ][{contextUsage}][ · ][{cost}]       ← status[] = [formatContextUsage(tokens, percent), money.format(cost)]
```

* `{N subagent(s)}` — count of `session.family(id)` entries ≠ self with `status === "running"`; plural `s` when `!= 1`.
* `{N shell(s)}` — `data.shell.list(location)` filtered by `metadata.sessionID === sessionID`.
* `formatContextUsage(tokens, percent)` = `Locale.number(tokens)` + ` (percent%)` when percent is known;
  `Locale.number`: `>= 999950 → (n/1e6).toFixed(1)+"M"`, `>= 1000 → (n/1e3).toFixed(1)+"K"`, else `n.toString()`
  → e.g. **`122.2K (61%)`**.
* Cost only when `cost > 0`, formatted with `Intl.NumberFormat("en-US",{style:"currency",currency:"USD"})` → **`$0.42`**.
* The subagent/shell group is a click target → `keymap.dispatch("session.child.first", subagents ? "subagents" : "shell")`,
  and its leading shortcut label (when bound) renders in `text.base`.

**Right group** — `{shortcut("agent.cycle")} agents` and `{shortcut("command.palette.show")} commands`,
shortcut in `text.base`, word in `text.muted`; whole thing `text.action.primary.disabled` + `DIM`
when the prompt is not interactive.

**Layout algorithm** (`promptFooterLayout`), exact:

```
usage     = status.join(" · ")
shortcuts = ["{agent.cycle} commands"…].join(" · ")      // built from command.palette.show
available = max(0, (termWidth - 8) - min(28, floor((termWidth - 8) / 2)))
if usage && shortcuts && width(usage + " · " + shortcuts) <= available → both
else if usage && width(usage) <= available                                 → usage only
else if shortcuts && width(shortcuts) <= available                         → shortcuts only
else                                                                      → neither
```

**Special cases:**

* `status().length === 0` → `{usage: false, shortcuts: termWidth >= 44}`. Because the `agents` hint lives
  in the *second* `<Match>` (which only fires when the first match — goal / live subagents+shells / status —
  is false), **`⇧tab agents` is shown only when there is no usage status, no goal and no running
  subagent/shell**, and only when `width >= 44`. `ctrl+p commands` sits outside both matches, so it shows
  whenever `showDetails && layout().shortcuts`.
* **Shell mode** replaces the whole footer with `esc ` (`text.base`) + `exit shell mode` (`text.muted`),
  shortened to **`shell`** when `termWidth < 44`: i.e. `esc shell`.
* `showDetails = (store.interrupt === 0) || termWidth >= 80` — while an interrupt is armed, context/cost
  details are hidden on narrow terminals.

### 7.5 Running state / interrupt

* Left cell `marginLeft 1`: `<spinner color={agentColor ?? border.base} frames={blocks} interval={40}/>`.
  Frames come from `createFrames({color, style:"blocks", inactiveFactor:0.6, minAlpha:0.3})` and
  `createColors(...)` with the same args. When `config.animations === false` the fallback is the literal
  muted text **`[⋯]`**.
* Then `PromptInterruptStatus`: `esc ` in `text.base` + `interrupt` in `text.muted`; when armed →
  `again to interrupt` in `armedColor = level==0 || !flash ? warning : tint(warning, flash, level)`
  with `flash = theme.decrease(text.feedback.warning.base, 2)` and an ignition animation
  (`jump 0.75 → animate 0`).
* **Double-Esc contract** (`prompt/index.tsx:502–533`, command `session.interrupt`, `palette: undefined`):
  1. First press: `interrupt += 1`, `setTimeout(..., 5000)` resets it to 0 → label flips to `again to interrupt`.
  2. Second press within 5 s (`interrupt >= 2`): `client.api.session.interrupt({sessionID, resume: true})`,
     then `interrupt = 0`.
  3. Shell mode: the same key exits shell mode instead (`mode = "normal"`).
  4. Guarded by `!auto?.visible` and `input.focused`.

### 7.6 Idle footer left cell

`footerLocation` = pending directory, else (no session) `currentLocation.ref ?? data.location.default()`,
else the session's location **only when `status() === "idle"`**. Label =
`abbreviateHome(directory, paths.home)` + `":" + branch` when a branch exists → `~/src/repo:main`.
Rendered truncated via `truncateFilePath(label, availableWidth)`, `text.muted` (base when the
working-directory action is hovered). While a worktree move is in flight the left cell instead shows a
`hue.accent[500]` spinner + `"." * creatingDots`, or the literal **`(new worktree)`** in `hue.accent[500]`.

### 7.7 Other composer behaviors (confirmations)

Multi-line: `shift+return`, `ctrl+return`, `alt+return`, `ctrl+j`. Submit `return`. History `↑`/`↓`
when the buffer is empty. `ctrl+c` clears (`prompt.clear`). Queue `leader return`. External editor
`leader e` (`prompt.editor`). Images viewer `leader i`. `prompt.paste` default is the object form
`{"key":"ctrl+v","preventDefault":false}`.

---

## 8. Assistant footer

Source: `routes/session/index.tsx:2016–2074` (`AssistantFooter`), rendered at line 1726.

### 8.1 Exact render

```tsx
<box paddingLeft={3} marginTop={retry || (error && !interrupted) ? 1 : 0}>
  <text>
    <span fg={error ? text.muted : local.agent.color(agent)}>{Locale.titlecase(agent)}</span>
    {width >= 28            && <span fg={muted}> · {model}</span>}
    {duration && (width < 28 || width >= 36) && <span fg={muted}> · {Locale.duration(duration)}</span>}
    {config.session.tps && tps && <span fg={muted}> · {tps.toFixed(1)} tok/s</span>}
    {interrupted            && <span fg={muted}> · interrupted</span>}
  </text>
</box>
```

* Separator is exactly `" · "` (space, U+00B7, space).
* `model` = the catalog `model.name`, falling back to `"{providerID}/{id}"`.
* `interrupted` ⇔ `message.error?.message === "Step interrupted"`.
* `duration` = `SessionTurn.duration(messages, message, {index, legacy})`;
  `tps` = `SessionTurn.tokensPerSecond(...)`; both from `SessionTurn`.
* **Width breakpoints (exact):** model hidden below **28**; duration hidden in the band
  **28 ≤ width < 36**; below 28 the duration *is* shown (that's what `width < 28 || width >= 36` means).
* **`ttft` does not exist** — searched `packages/tui` and `packages/util`: **not found**. Do not render it.

### 8.2 Duration format — `packages/util/src/duration.ts` (byte-exact)

```ts
export function duration(input: number) {
  if (input <    1000) return `${input}ms`                              // 850ms
  if (input <   60000) return `${(input / 1000).toFixed(1)}s`           // 12.4s
  if (input < 3600000) return `${m}m ${s}s`                             // 3m 5s
  if (input <86400000) return `${h}h ${m}m`                             // 2h 10m
  return `${d}d ${h}h`                                                  // 1d 4h
}
```
with `m = floor(input/60000)`, `s = floor((input%60000)/1000)`,
`h = floor(input/3600000)`, `m = floor((input%3600000)/60000)`,
`d = floor(input/86400000)`, `h = floor((input%86400000)/3600000)`.

### 8.3 Error line (delta to §4.2)

Rendered **above** the footer, in its own `<box paddingLeft={3}>`:
`<text fg={theme.text.feedback.error.base}>Error: {errorMessage(error)}</text>`
— literal prefix **`Error: `**. Two special cases replace it: `error.type === "provider.free-limit"` →
`FreeLimitNotice`, and `useLimitNotice(...)` → `UsageLimitNotice`. `AssistantRetry` renders
`message.retry` when present. `marginTop 1` separates the footer from any of these.

### 8.4 Related transcript rows (for parity)

| Row | Exact shape |
|---|---|
| Reasoning (delta to §4.3) | `Thought: {title} · {duration}` in `text.feedback.warning.base`; `duration` shown only when the part `isDone()`; click toggles collapse; `session.toggle.thinking` default **`none`** |
| Location switch | paddingLeft 3, `↳ Moved to ` muted + directory in `feedback.info.base` |
| Tool inline row | paddingLeft 3, icon column width **`INLINE_TOOL_ICON_WIDTH = 2`**, glyph `✓` / `✗` / spinner per tool, pending row uses the spinner's text |
| Group view footer | ` · {Locale.duration(end - start)}` appended to the group line when completed |
| User message | `background.raised.base` box, padding 2/1, hover → `decrease(raised.base)`, click opens `DialogMessage` |
| Tool images | `SessionImages` per §5.2 (`paddingLeft 3`) |

---

## 9. Keybinds

**Correction to `tui-spec.md` §3 in one line: the command IDs are wrong.** Current IDs are
**dotted lowercase** (`command.palette.show`, not `command_list`; `session.sidebar.toggle`, not
`sidebar_toggle`; `prompt.editor`, not `editor_open`; `theme.switch`, not `theme_list`;
`opencode.status`, not `status_view`; `open.menu`, not `open_menu`). Leader default **`ctrl+x`**,
referenced in other bindings as `<leader>`; `leader.timeout` default **1500** ms (docs examples use
1500/2000). Config lives in **`cli.json`** under `keybinds`. A binding may be a string, a
comma-separated string, an array, an object `{key, preventDefault}`, `"none"`, or `false`.
**Unknown command IDs are ignored.**

The tables below are transcribed from `https://opencode.ai/docs/cli/keybinds` (authoritative) and
cross-checked against `packages/tui/src/config/keybind.ts`.

### 9.1 Application

| ID | Default | Description |
|---|---|---|
| `leader` | `ctrl+x` | Leader key for keybind combinations |
| `app.exit` | `ctrl+c,ctrl+d,<leader>q` | Exit the application |
| `app.debug` | `none` | Toggle debug panel |
| `app.console` | `none` | Toggle console |
| `app.scrap` | `none` | Open scrap screen |
| `app.toggle.animations` | `none` | Toggle animations |
| `app.toggle.file_context` | `none` | Toggle file context |
| `app.toggle.diffwrap` | `none` | Toggle diff wrapping |
| `app.toggle.paste_summary` | `none` | Toggle paste summary |
| `command.palette.show` | `ctrl+p` | List available commands |
| `help.show` | **`none`** | Open help dialog |
| `docs.open` | `none` | Open documentation |
| `opencode.settings` | `none` | Open settings |
| `server.pair` | `none` | Pair device |
| `service.restart` | `none` | Restart service |
| `location.reload` | `none` | Reload configuration |
| `app.clear` | `ctrl+l` | Clear the screen (**opencode mini only**) |

### 9.2 Appearance & navigation

| ID | Default | Description |
|---|---|---|
| `prompt.editor` | `<leader>e` | Open external editor |
| `theme.switch` | **`none`** | List available themes |
| `theme.switch_mode` | `none` | Switch between light and dark theme mode |
| `theme.mode.lock` | `none` | Lock or unlock theme mode |
| `session.sidebar.toggle` | `<leader>b` | Toggle sidebar |
| `pane.focus.left` | `<leader>left` | Focus session pane |
| `pane.focus.right` | `<leader>right` | Focus right pane |
| `terminal.select` | `<leader>down` | Select terminal |
| `terminal.toggle` | `<leader>t` | Toggle terminal pane |
| `terminal.close` | `<leader>up` | Close terminal pane |
| `session.toggle.scrollbar` | `none` | Toggle session scrollbar |
| `opencode.status` | `<leader>s` | View status |
| `opencode.debug` | `none` | View debug info |

### 9.3 Sessions

| ID | Default |
|---|---|
| `session.export` | `<leader>x` |
| `session.copy` / `session.copy.id` | `none` / `none` |
| `session.move` | `none` |
| `session.new` | `<leader>n` |
| `session.list` | `<leader>l` |
| `open.menu` | `ctrl+o` |
| `session.tab.next` | `ctrl+tab,alt+down` |
| `session.tab.previous` | `ctrl+shift+tab,alt+up` |
| `session.tab.history.back` / `.forward` | `none` / `ctrl+i` |
| `session.tab.next_unread` / `.previous_unread` | `alt+shift+down` / `alt+shift+up` |
| `session.tab.close` | `<leader>w` |
| `session.tab.reopen` | `ctrl+shift+t` |
| `session.timeline` | `<leader>g` |
| `session.fork` | `none` |
| `session.rename` | `ctrl+r` |
| `session.delete` | `ctrl+d` |
| `session.interrupt` | `escape` |
| `session.background` | `ctrl+b` |
| `session.compact` | `<leader>c` |
| `session.aside` / `session.cd` | `none` / `none` |
| `session.queued_prompts` | `<leader>q` |
| `queued_prompt.undo` / `.delete` | `ctrl+u` / `ctrl+d` |
| `session.toggle.exploration_grouping` | `none` |
| `session.verbosity.cycle` | `none` |
| `session.child.first` / `.next` / `.previous` / `session.parent` | `down` / `right` / `left` / `up` |
| `session.pin.toggle` | `ctrl+f` |
| `session.quick_switch.1..9` | `<leader>1` … `<leader>9` |
| `session.tab.select.1..9` | `<leader>1,ctrl+1` … `<leader>9,ctrl+9` |
| `session.tab.select.10` | `<leader>0,ctrl+0` |

### 9.4 Models & agents

| ID | Default |
|---|---|
| `stash.delete` | `ctrl+d` |
| `model.dialog.provider` | `ctrl+a` |
| `model.dialog.favorite` | `ctrl+f` |
| `model.dialog.usage` | `ctrl+u` |
| `model.list` | `<leader>m` |
| `model.cycle_recent` / `model.cycle_recent_reverse` | `f2` / `shift+f2` |
| `model.cycle_favorite` / `_reverse` | `none` / `none` |
| `mcp.list` | `none` |
| `usage.show` | `none` |
| `provider.connect` | `none` |
| `agent.list` | `<leader>a` |
| `agent.cycle` / `agent.cycle.reverse` | `shift+tab` / `none` |
| `variant.cycle` / `variant.list` | `ctrl+t` / `none` |

### 9.5 Session navigation

| ID | Default |
|---|---|
| `session.page.up` / `.down` | `pageup,ctrl+alt+b` / `pagedown,ctrl+alt+f` |
| `session.line.up` / `.down` | `ctrl+alt+y` / `ctrl+alt+e` |
| `session.half.page.up` / `.down` | `ctrl+alt+u` / `ctrl+alt+d` |
| `session.first` | `ctrl+g,home,alt+home` |
| `session.last` | `ctrl+alt+g,end` |
| `session.message.next` / `.previous` | `none` / `none` |
| `session.message.user.next` / `.previous` | `none` / `none` |
| `session.messages_last_user` | `alt+end` |
| `messages.copy` | `<leader>y` |
| `session.undo` / `session.redo` | `<leader>u` / `<leader>r` |
| `session.toggle.thinking` | `none` |

### 9.6 Prompt commands

| ID | Default |
|---|---|
| `prompt.submit` | `none` |
| `prompt.queue` | `<leader>return` |
| `prompt.editor_context.clear` | `none` |
| `prompt.images.view` | `<leader>i` |
| `prompt.skills` / `prompt.stash` / `prompt.stash.pop` / `prompt.stash.list` | `none` |

### 9.7 Input editing

| ID | Default |
|---|---|
| `prompt.clear` | `ctrl+c` |
| `prompt.paste` | `{"key":"ctrl+v","preventDefault":false}` |
| `input.submit` | `return` |
| `input.newline` | `shift+return,ctrl+return,alt+return,ctrl+j` |
| `input.move.left` / `.right` | `left,ctrl+b` / `right,ctrl+f` |
| `input.move.up` / `.down` | `up` / `down` |
| `input.select.left` / `.right` / `.up` / `.down` | `shift+left` / `shift+right` / `shift+up` / `shift+down` |
| `input.line.home` / `.end` | `ctrl+a` / `ctrl+e` |
| `input.select.line.home` / `.end` | `ctrl+shift+a` / `ctrl+shift+e` |
| `input.visual.line.home` / `.end` | `alt+a` / `alt+e` |
| `input.select.visual.line.home` / `.end` | `alt+shift+a` / `alt+shift+e` |
| `input.buffer.home` / `.end` | `none` / `none` |
| `input.select.buffer.home` / `.end` | `shift+home` / `shift+end` |
| `input.delete.line` | `ctrl+shift+d` |
| `input.delete.to.line.end` / `.start` | `ctrl+k` / `ctrl+u` |
| `input.backspace` | `backspace,shift+backspace` |
| `input.delete` | `ctrl+d,delete,shift+delete` |
| `input.undo` / `input.redo` | `ctrl+-,super+z` / `ctrl+.,super+shift+z` |
| `input.word.forward` / `.backward` | `alt+f,alt+right,ctrl+right` / `alt+b,alt+left,ctrl+left` |
| `input.select.word.forward` / `.backward` | `alt+shift+f,alt+shift+right` / `alt+shift+b,alt+shift+left` |
| `input.delete.word.forward` / `.backward` | `alt+d,alt+delete,ctrl+delete` / `ctrl+w,ctrl+backspace,alt+backspace` |
| `input.select.all` | `super+a` |
| `prompt.history.previous` / `.next` | `up` / `down` |

### 9.8 Composer (subagent / shell / terminal pickers)

| ID | Default |
|---|---|
| `composer.subagent.up` / `.down` / `.select` / `.interrupt` | `up` / `down` / `return` / `ctrl+d` |
| `composer.shell.up` / `.down` / `.select` / `.kill` | `up` / `down` / `return` / `ctrl+d` |
| `composer.terminal.up` / `.down` / `.select` | `up,k` / `down,j` / `return` |

### 9.9 Dialogs & autocomplete

| ID | Default |
|---|---|
| `dialog.select.prev` / `.next` | `up,ctrl+p` / `down,ctrl+n` |
| `dialog.select.page_up` / `.page_down` | `pageup` / `pagedown` |
| `dialog.select.home` / `.end` | `home` / `end` |
| `dialog.select.submit` / `dialog.prompt.submit` | `return` / `return` |
| `dialog.integration.rename` / `.delete` / `.copy` | `ctrl+r` / `ctrl+d` / `tab` |
| `dialog.worktree.generate` | `tab` |
| `dialog.move_session.new` / `.move` / `.delete` / `.refresh` | `ctrl+a` / `ctrl+m` / `ctrl+d` / `ctrl+r` |
| `prompt.autocomplete.prev` / `.next` | `up,ctrl+p` / `down,ctrl+n` |
| `prompt.autocomplete.hide` / `.select` / `.complete` | `escape` / `return` / `tab` |
| `permission.prompt.fullscreen` | `ctrl+f` |
| `plugins.toggle` | `return` |
| `dialog.mcp.toggle` / `dialog.plugins.error` | `space` / `space` |
| `dialog.plugins.update` / `.check` | `ctrl+u` / `ctrl+r` |

### 9.10 Diff viewer (replaces `tui-spec.md` §3.6)

| ID | Default | | ID | Default |
|---|---|---|---|---|
| `diff.open` | `none` | | `diff.next_hunk` / `.previous_hunk` | `]` / `[` |
| `diff.close` | `escape,q` | | `diff.next_file` / `.previous_file` | `n,alt+down` / `p,alt+up` |
| `diff.down` / `.up` | `j,down` / `k,up` | | `diff.toggle_file_tree` | `b` |
| `diff.page.down` / `.up` | `pagedown,ctrl+f` / `pageup,ctrl+b` | | `diff.single_patch` | `s` |
| `diff.half_page.down` / `.up` | `ctrl+d` / `ctrl+u` | | `diff.switch_source` | `d` |
| `diff.first` / `.last` | `gg,home` / `shift+g,end` | | `diff.toggle_view` | `v` |
| `diff.toggle_fullscreen` | `f` | | `diff.mark_reviewed` | `m` |
| `diff.help` | `?,shift+?,shift+/` | | | |

**Corrections to §3.6:** `enter`/`space` toggle, `E` expand all, and `tab` (switch focus) are **not**
bindings — `diff.expand`, `diff.expand_all`, `diff.collapse`, `diff.switch_focus` and `diff.toggle` are
retired config entries that "are still accepted, but no longer register shortcuts".
`diff.page.down` is `pagedown,ctrl+f` (not `ctrl+f` alone), `diff.half_page.down` is `ctrl+d`.
There is **no** focus switch: clicking files/folders is the only way to move focus to the tree, and the
mouse wheel scrolls it. `alt+down`/`alt+up` navigate **files** inside the diff viewer, not session tabs.
`m` marks reviewed; right-click a file → `Mark complete` / `Mark incomplete`.

### 9.11 Terminal, plugins, which-key (new groups for the clone)

| ID | Default |
|---|---|
| `terminal.suspend` | `ctrl+z` |
| `terminal.title.toggle` | `none` |
| `plugins.list` | `none` |
| `which-key.toggle` | `ctrl+alt+k` |
| `which-key.layout.toggle` | `ctrl+alt+shift+k` |
| `which-key.pending.toggle` | `ctrl+alt+shift+p` |
| `which-key.group.previous` / `.next` | `ctrl+alt+left,ctrl+alt+[` / `ctrl+alt+right,ctrl+alt+]` |
| `which-key.scroll.up` / `.down` | `ctrl+alt+up,ctrl+alt+p` / `ctrl+alt+down,ctrl+alt+n` |
| `which-key.page.up` / `.down` | `ctrl+alt+pageup` / `ctrl+alt+pagedown` |
| `which-key.home` / `.end` | `ctrl+alt+home` / `ctrl+alt+end` |

---

## 10. Misc

### 10.1 Terminal title — `app.tsx:643–659`

```
home / no title     → "OpenCode"
session with title  → `OC | ` + (len > 40 ? title.slice(0,37) + "…" : title)
route.name fallback → `OC | ` + route.data.name
disabled            → "" (empty string clears it)
```
Toggle command: `terminal.title.toggle` (default `none`); palette title
`Disable terminal title` / `Enable terminal title`; setting keyword `terminal title`, `tab title`.
`opencode mini` shares the same helper (`mini/runtime.lifecycle.ts:180`) and additionally resets to `""`
on Linux (`platform === "linux"`).

### 10.2 Degradation breakpoints (compile these as named constants)

| Condition | Effect |
|---|---|
| `height < 12` | logo hidden |
| `width < 22` | logo = `go.right[1:]`, bold |
| `width < 44` | logo = muted `logo.left[1:]` + bold `logo.right`; home padding 1; composer padding 1; metadata drops `agent` and `provider`; footer shortcuts need `width >= 44`; placeholder width `termWidth - 5`; shell footer text `shell` |
| `width < 28` | assistant footer drops the model |
| `28 ≤ width < 36` | assistant footer drops the duration |
| `width >= 44` | home footer row possible; `⇧tab agents` hint possible |
| `width >= 64` | home footer shows `/mcps` + version |
| `width >= 80` | composer `showDetails` survives an armed interrupt; home footer `/plugins` needs 80 |
| `height >= 14 && width >= 44` | home footer row rendered |
| `height < 16` | home footer `paddingTop/Bottom 0` |
| `height >= 22` | sidebar "Getting started" card possible |
| `prompt.footer` layout | `available = max(0, (w-8) - min(28, floor((w-8)/2)))` |

### 10.3 Mouse / selection / animations / scrollbars

* Mouse: `config.mouse`, `targetFps 60`, kitty keyboard protocol supported **(inferred: enable when
  the terminal reports support, else fall back to legacy mouse)**.
* `copy_on_select` default **`manual`** on `win32` **(inferred: same default elsewhere)**; dialogs keep
  the release guard so a click that only selects does not activate the row.
* Animations default **on** (`config.animations ?? true`); `app.toggle.animations` (default `none`)
  flips them; when off, spinners are replaced by static text (`[⋯]`) and the entry splash animation is skipped.
* Scrollbars: base `#606060` dark / `#a0a0a0` light; hidden inside dialogs (`scrollbarOptions.visible: false`);
  `session.toggle.scrollbar` (default `none`) toggles the transcript scrollbar.
* Spinner frames (shared): `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`; tab spinner default `dots`; composer running spinner is
  `style:"blocks"` at **40 ms**.
* Crash screen exists (`component/error-component.tsx`): header **`OpenCode crashed`**, body
  `OpenCode {version}`, and it builds a markdown report with `**Error:**` / `**Stack trace:**` for
  automatic reporting.
* Toasts: default duration **5000 ms**; `Nothing to undo` uses **3000 ms** with `variant:"error"`.

### 10.4 Theme / config keys referenced by these topics

| Key | Type / default | Meaning |
|---|---|---|
| `mini.splash` | `"show" \| "hide"`, default `"show"` | entry/exit splash banners (mini only) |
| `animations` | bool, default `true` | global animation toggle |
| `tabs.indicators` | `"numbers" \| "status"` | tab indicator mode |
| `session.tps` | bool | show `· X.X tok/s` in the assistant footer |
| `session.image_preview` | bool | transcript/tool image thumbnails |
| `prompt.image_preview` | bool | composer attachment thumbnails |
| `mouse` | object (`targetFps: 60`) | mouse support |
| `keybinds` + `leader` | object / `"ctrl+x"` | see §9 |
| `diffs.source` | enum | initial diff review scope (`diff.switch_source` remembers until exit) |

---

## 11. Implementation checklist (Go + Bubble Tea)

Existing files in this repo: `internal/tui/{tui.go,update.go,view.go}`, `internal/cli/{cli.go,run.go,tui.go,api.go}`.
Everything marked **NEW** is a proposed new file; everything else is an edit to an existing file.
`internal/tui/*.go` = `package tui`, `internal/cli/*.go` = `package cli`.

| # | Spec element | Spec § | OpenCode source | Target file | Concrete task |
|---|---|---|---|---|---|
| 1 | Logo art + cell encoding | 1.2–1.3 | `logo.ts`, `component/logo.tsx` | **NEW** `internal/tui/logo.go` | Encode `logo`/`go` as `[][]rune`; renderer: `_`→space-on-shadow, `^`→`▀` fg+shadow, `~`→`▀` shadow, `,`→`▄` shadow; `shadow = mix(bg, fg, 0.25)` |
| 2 | Logo responsive tiers | 1.4 | `component/logo.tsx:54` | `internal/tui/logo.go` | Predicate chain `h<12 / w<22 / w<44 / else` exactly as §1.4 |
| 3 | Home route layout | 1.5 | `routes/home.tsx` | **NEW** `internal/tui/home.go` | Centered column, padding `(w<44?1:2)`, 3-row gap, logo, 1-row gap, update notice, composer `maxWidth 75`, `home.footer` slot |
| 4 | Update notification | 1.6 | `routes/home.tsx:125` | `internal/tui/home.go` | 3 states with the exact strings; width = measured logo width |
| 5 | Home footer | 1.7 | `feature-plugins/home/footer.tsx` | `internal/tui/home.go` | Gate `h>=14 && w>=44`; `⊙ N MCP [ /mcps@w≥64]`, `⊙ N plugins failed [ /plugins@w≥80]`, version right `@w≥64`; `gap 2`, padding 2 |
| 6 | Startup loading pill | 1.8 | `component/startup-loading.tsx` | `internal/tui/tui.go` | `zIndex`-equivalent (render last), 500 ms delay / 3000 ms hold, `Loading plugins…` → `Finishing startup…` |
| 7 | Slash command registry | 2.2–2.3 | `app.tsx`, `routes/session/*`, `core/plugin/command.ts` | **NEW** `internal/tui/slash.go` | 38 built-ins `{name, aliases, title, arguments, run}` + `/init`, `/review` + MCP prompts + user `.md`/`.json` commands; `arguments:true` inserts `/name ` |
| 8 | Custom command loading | 2.3 | docs `/docs/commands` | **NEW** `internal/cli/commands.go` | `~/.config/opencode/commands/**.md`, `.opencode/commands/**.md`, nested `/` names, frontmatter `description/agent/model/subagent`, `$ARGUMENTS`/`$1…`/fallback-append, `` !`cmd` `` shell splice, precedence + hot reload |
| 9 | Slash autocomplete | 2.4 | `component/prompt/autocomplete.tsx` | **NEW** `internal/tui/autocomplete.go` | `┃` rail, `display` padded to `maxDisplayLen+2`, muted description, `localeCompare` sort, `↑↓`/`ctrl+p/n`, `enter`/`tab`/`esc` |
| 10 | Command palette | 3 | `component/command-palette.tsx` | **NEW** `internal/tui/palette.go` | Title `Commands`, placeholder `Search commands`, `filterThreshold 0.7`, `Suggested` category prefix when unfiltered, settings entries with `Settings · {category}` footer, right-aligned keybind label, `run()` on select |
| 11 | Auth CLI | 4.1 | `cli/src/commands/commands.ts` | `internal/cli/cli.go` + **NEW** `internal/cli/auth.go` | Subcommands `auth list|login|logout|export|import|switch` with the exact descriptions and args of §4.1; keep `models/stats/mcp/plugins/sessions/service/reload/pair/run/mini/debug/acp/serve/update/uninstall` |
| 12 | `/connect` dialog | 4.2 | `component/dialog-integration.tsx` | **NEW** `internal/tui/connect.go` | Titles `Connect an integration` / `Connect {name}` / `{name}` / `Rename account`; methods `key` (placeholder `API key`, inline error), `oauth` (`Starting authorization…`/`Connecting…`/`Opening link…`), `command` (`Starting command…`/`Waiting for command to finish…`), `external`; success toast `Connected {name}`; `Connected accounts` category |
| 13 | First-run onboarding | 4.3 | `dialog-model-free.tsx`, `sidebar/footer.tsx` | `internal/tui/home.go`, **NEW** `internal/tui/sidebar.go` | `Free models` / `Connect a provider for access to more models`; sidebar card `#1e1e1e`, `⬖`, `Getting started` + `✕`, both body strings, `Connect provider … /connect`, gate `h>=22` + not-dismissed + no connected provider |
| 14 | Clipboard / paste | 5.1 | `clipboard.ts` | **NEW** `internal/tui/images.go` | Prefer `image/png` then `text/plain`; 0-byte text → treat as image paste; map all 4 error statuses incl. `limit-exceeded` → `RangeError`; write strips NUL |
| 15 | Composer thumbnails | 5.2 | `prompt/index.tsx:1698` | `internal/tui/images.go`, `internal/tui/composer.go` | `h = max(4,min(8,floor(H/4)))`, `w = 2h`, first 3, `+N more` in an 8-wide tile, `No preview` fallback, gated by `prompt.image_preview` |
| 16 | Transcript thumbnails | 5.2 | `routes/session/index.tsx:2662` | `internal/tui/messages.go` | Same formula, `paddingLeft 3`, gated by `session.image_preview` |
| 17 | Image viewer dialog | 5.3 | `component/dialog-image-preview.tsx` | `internal/tui/images.go` | Header `Image {n} of {m}`, `← previous` / `→ next` chips, `left`/`right` keys, `No preview` fallback |
| 18 | Tab constants + adaptive widths | 6.1 | `session-tabs-model.ts` | **NEW** `internal/tui/tabs.go` | 22/32/8/`len+2`; `roomy` formula; slack distribution floored at `MIN` |
| 19 | Overflow indicators | 6.2 | `session-tabs.tsx:1546,1752` | `internal/tui/tabs.go` | `‹N` left, ` N›` right |
| 20 | Marquee + fade | 6.3 | `session-tabs.tsx`, `util/marquee.ts` | `internal/tui/tabs.go` | 600 ms delay / 80 ms step; fade `0.2 + 0.24*(i)` over 4 cells (`0.20 … 0.92`) |
| 21 | Tab indicators | 6.4 | `session-tabs.tsx:131` | `internal/tui/tabs.go` | Priority `numbers → ! → ? → spinner → + → unread marker`; colors `#9d7cd8` unread / `#e06c75` error-unread / `#fab283` running / `hue.accent` attention; hover `✕` at right:1; middle-click close; right-click menu; drag reorder |
| 22 | Tab strip + vertical rail | 6.4 | `session-tabs.tsx` | `internal/tui/tabs.go` | Horizontal `height 1` + 3-wide `+` slot; vertical stride 3 / compact 2, `paddingY 1` |
| 23 | Sidebar | 6.5 | `sidebar/*.tsx` | `internal/tui/sidebar.go` | Width 42, `#141414`; `Context` bold + `12,345 tokens` + `61% used` + `$0.42 spent`; `Limit` + `$X remaining` (error when danger); footer `FilePath` `maxWidth 38` as `~/path:branch` |
| 24 | Composer shell | 7.1–7.2 | `component/prompt/index.tsx` | **NEW** `internal/tui/composer.go` | `╹` left border (space when bg alpha 0), top `▀` shade row, `bg = decrease(#141414)`, `maxHeight = max(6, floor(H/3))`, padding `w<44?1:2` |
| 25 | Placeholder | 7.1 | `prompt/index.tsx:1614` | `internal/tui/composer.go` | `Ask anything… "{hint}"` / `Run a command… "{hint}"`; width `(w<44 ? w-5 : min(75,w-4)-5)` then `takeWidth` + `trimEnd`; hints `Fix a TODO in the codebase` / `What is the tech stack of this project?` / `Fix broken tests` and `ls -la` / `git status` / `pwd` |
| 26 | Metadata row | 7.3 | `component/prompt/metadata.tsx` | `internal/tui/composer.go` | Budget `w - (w<44?9:13)`; 5-candidate drop order + truncate fallback; colors per §7.3 table; `auto` only in autoaccept; `Shell` fallback in shell mode |
| 27 | Footer status/hints | 7.4 | `feature-plugins/prompt/footer.tsx` | `internal/tui/footer.go` | `N subagents · N shells` + goal + `122.2K (61%) · $0.42` left; `⇧tab agents` (only if no status/goal/live children and `w>=44`) + `ctrl+p commands` right; layout fn of §7.4; shell → `esc exit shell mode` / `esc shell` |
| 28 | Running state + double-Esc | 7.5 | `prompt/index.tsx:502,1921` | `internal/tui/composer.go` | Blocks spinner @ 40 ms in agent color (fallback `[⋯]`); `esc interrupt` → `esc again to interrupt`; 5 s window; 2nd press interrupts with `resume: true`; shell mode consumes Esc first |
| 29 | Idle location label | 7.6 | `prompt/index.tsx:1627` | `internal/tui/composer.go` | `~/path:branch`, muted, truncated; `(new worktree)` + accent spinner during a move |
| 30 | Assistant footer | 8.1 | `routes/session/index.tsx:2016` | **NEW** `internal/tui/footer.go` | `paddingLeft 3`; `Titlecase(agent)` in agent color (muted on error) + ` · model` `@w≥28` + ` · dur` `@w<28‖w≥36` + ` · X.X tok/s` if `session.tps` + ` · interrupted`; no `ttft` |
| 31 | Duration formatter | 8.2 | `packages/util/src/duration.ts` | `internal/tui/footer.go` | Exact thresholds `1000/60000/3600000/86400000`, `toFixed(1)` for seconds |
| 32 | Error / retry line | 8.3 | `AssistantFooter` | `internal/tui/messages.go` | `Error: {msg}` in `#e06c75` at `paddingLeft 3`, `marginTop 1` when retry/error present |
| 33 | Tool inline row | 8.4 | `InlineToolRow` | **NEW** `internal/tui/tools.go` | `paddingLeft 3`, 2-cell icon column, `✓`/`✗`/spinner, pending spinner text |
| 34 | Reasoning row | 8.4 | `message-parts.tsx` | `internal/tui/tools.go` | `Thought: title · dur` in `#f5a742`, duration only when done, click-to-toggle |
| 35 | Keymap engine | 9 | `config/keybind.ts`, docs `/cli/keybinds` | **NEW** `internal/tui/keymap.go` | Dotted IDs; string / comma-list / array / `{key,preventDefault}` / `"none"` / `false`; `<leader>` expansion; `leader.timeout 1500`; unknown IDs ignored; all tables §9.1–9.11 as defaults |
| 36 | Chord / which-key | 9.11 | `config/keybind.ts` | `internal/tui/keymap.go` | Pending-leader state machine; which-key panel `ctrl+alt+k` (+ layout/pending/group/scroll/page/home/end) |
| 37 | Diff viewer keys | 9.10 | docs `/cli/keybinds` | `internal/tui/diff.go` (edit) | Exact table §9.10; drop `E`/`tab`/`enter`/`space` from §3.6; `alt+down/up` = files here, not tabs |
| 38 | Terminal title | 10.1 | `app.tsx:643` | `internal/tui/tui.go` | `OC \| ` + 40-char truncate (`slice(0,37)+"…"`), `OpenCode` on home, `""` when disabled |
| 39 | Degradation table | 10.2 | several | `internal/tui/layout.go` (edit) | Named constants for every breakpoint in §10.2; use them in logo, composer, footer, home, assistant footer |
| 40 | Mouse / copy-on-select | 10.3 | `config` | `internal/tui/update.go` (edit) | 60 fps mouse, kitty protocol probe, `copy_on_select` default `manual` on Windows, dialog release guard |
| 41 | Animations toggle | 10.3 | `config.animations` | `internal/tui/update.go` (edit) | `app.toggle.animations`; static `[⋯]` spinner fallback; spring width approximation for tabs **(inferred)** |
| 42 | Config keys | 10.4 | `config/index.tsx` | `internal/config/config.go` (edit) | Add `mini.splash`, `tabs.indicators`, `session.tps`, `session.image_preview`, `prompt.image_preview`, `mouse`, `keybinds`, `leader`, `diffs.source` |
| 43 | CLI wiring for `/…` commands | 2, 4 | `cli/src/commands/*` | `internal/cli/tui.go` (edit) | Pass the loaded command registry + auth state into the TUI model; `run`/`models`/`api`/`setup` unchanged |

**Sequencing (inferred):** 35 → 1/2 → 39 → 24–29 → 30/31 → 18–22 → 23 → 7/9 → 10 → 12 → 14–17 → 38/40/41 → 43.
The keymap (35) and the degradation constants (39) are prerequisites for everything else because nearly
every component branches on a keybind label or a width breakpoint.

**Byte-exactness rules for the clone**

1. Keep every string in §2.2, §4.1–4.3, §5.3, §7.4–7.6, §8.1–8.3 byte-identical (including `…`, `·`, `⊙`, `⬖`, `✕`, `‹`, `›`, `↳`, `▀`, `╹`, `▲`/`△`).
2. Keep the hexes from `tui-spec.md` §2; this file adds only `tint(bg, fg, 0.25)` for the logo shadow and
   `decrease(raised.base)` / `decrease(warning, 2)` for the composer bg and interrupt flash.
3. Never invent commands: **`/share /cost /retry /switch /step /branch /auth /ttft /mcp` do not exist.**
4. `help.show`, `theme.switch`, `session.aside`, `session.cd`, `prompt.submit`, `usage.show`,
   `provider.connect`, `mcp.list`, `plugins.list`, `session.fork` all default to **`none`** — they are
   reachable from the palette/`/…` menu but not from a key.
