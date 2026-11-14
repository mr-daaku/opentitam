# OpenTitam

A local-first, terminal-based AI coding assistant — an OpenCode-style CLI rebuilt in
Go with **no built-in model catalog**. You bring your own OpenAI-compatible endpoint
(`baseURL` + `apiKey`); models are loaded dynamically from `GET {baseURL}/models`.

The PC is the backend: sessions, config, and approvals all live on your machine.
No cloud service, no telemetry, no hardcoded provider list.

## Features

- **Dynamic models only** — the model list always comes from your endpoint's
  `/models` API. Nothing is hardcoded.
- **Works with any OpenAI-compatible provider** — OpenAI, DeepSeek, OpenRouter,
  Ollama, LM Studio, Groq, Mistral, NVIDIA NIM, vLLM, …
- **Streaming chat with tool calling** — SSE streaming + a full agent loop
  (model ⇄ tools until the task is done).
- **Six coding tools** — `read`, `write`, `edit`, `glob`, `grep`, `shell`
  (PowerShell-aware on Windows).
- **Permission engine** — OpenCode-style `action / resource / effect` rules with
  `*` / `?` wildcards, last-match-wins, `.env → ask` by default, and an in-TUI
  approval prompt (*Allow once / Always allow / Reject*).
- **Terminal UI** — dark theme, session strip, scrollable transcript, composer
  with command palette, model picker, session sidebar, permission stack.
- **Local sessions** — list, resume, and persist conversations as JSON under your
  local data dir.
- **Context auto-trim** — history is trimmed by estimated tokens before overflow.

## Install

### From source (requires Go ≥ 1.24)

```sh
git clone https://github.com/mr-daaku/opentitam.git
cd opentitam
go build -o opentitam ./cmd/opentitam
```

Or straight from the repo:

```sh
go install github.com/mr-daaku/opentitam/cmd/opentitam@latest
```

## Quick start

```sh
# 1. Add a provider (base URL + API key) and fetch models
opentitam setup

# 2. List models loaded from your endpoint
opentitam models

# 3. Chat / code
opentitam              # interactive TUI
opentitam run "fix the failing test"   # headless one-shot
```

`setup` asks for three things — provider name, base URL, and API key (leave the
key empty for local servers like Ollama) — then immediately fetches the model list
from the endpoint and lets you pick a default.

## Configuration

Global config lives at `~/.config/opentitam/opentitam.json`
(override with `OPENTITAM_CONFIG_DIR`). Project-level configs are discovered in
`<repo>/opentitam.json` or `<repo>/.opencode/opentitam.json` (closest wins).

```jsonc
{
  "model": "openai/gpt-4o-mini",
  "providers": {
    "openai": {
      "baseURL": "https://api.openai.com/v1",
      "apiKey": "{env:OPENAI_API_KEY}"
    },
    "ollama": {
      "baseURL": "http://127.0.0.1:11434/v1",
      "apiKey": ""
    }
  },
  "permissions": [
    { "action": "shell", "resource": "git status *", "effect": "allow" },
    { "action": "shell", "resource": "git push *", "effect": "ask" }
  ]
}
```

- `{env:NAME}` and `{file:path}` references are expanded inside JSON strings.
- `//` comments and trailing commas are allowed (JSONC).
- Permission rules are evaluated in order; the **last match wins**. If nothing
  matches, the effect is `ask`.

Sessions are stored under `%LOCALAPPDATA%\opentitam\sessions` on Windows and
`~/.local/share/opentitam/sessions` elsewhere (override with `OPENTITAM_DATA_DIR`).

## Key bindings

| Key | Action |
|---|---|
| `enter` | Send message |
| `ctrl+j` / `alt+enter` | Newline in composer |
| `ctrl+x` then… `q` quit · `m` models · `n` new session · `l` sessions · `c` compact · `b` sidebar | Leader chords |
| `ctrl+p` | Command palette |
| `ctrl+o` | Sessions |
| `f2` / `shift+f2` | Cycle model next / previous |
| `esc` | Interrupt generation · close dialog |
| `ctrl+c` | Clear prompt (or quit when empty) |

## Slash commands

`/new` · `/sessions` · `/models` · `/compact` · `/clear` · `/help` · `/exit`

## Architecture

```
cmd/opentitam/        entry point
internal/cli/         run, models, setup, version subcommands
internal/config/      global + project config merge, {env:}/{file:} substitution
internal/provider/    dynamic GET /models, SSE chat client, tool-call merging
internal/agent/       system prompt, agent loop, context trim
internal/tools/       read, write, edit, glob, grep, shell
internal/permissions/ action/resource/effect rule engine
internal/session/     local JSON session store
internal/tui/         Bubble Tea terminal UI (design spec: docs/tui-spec.md)
mockserver/           fake OpenAI-compatible server used by the test suite
```

## Development

```sh
go vet ./...
go test ./...     # unit + integration tests against the local mock server
go build ./...
```

The integration tests run a full round trip (user prompt → tool call → tool
execution → final answer) against `mockserver/`, so no network or API key is
needed to develop.

## License

[MIT](LICENSE)
