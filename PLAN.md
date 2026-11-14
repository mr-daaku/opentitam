# OpenTitam — Plan (OpenCode-compatible CLI, branding: OpenCode → OpenTitam)

## Goal
A terminal AI coding CLI that mirrors OpenCode's system design, renamed to **OpenTitam**,
with one key difference: **no built-in/official model catalog**. The user supplies only
`baseURL` + `apiKey`; models are loaded dynamically from `GET {baseURL}/models`.

## Design decisions
1. **Go + Bubble Tea** — single binary, PC is the backend (no cloud services).
2. **Location** — new subfolder `OpenTitam/opentitam/`; the archived parent repo is untouched.
3. **Providers** — OpenAI-compatible only (`/models`, `/chat/completions` with SSE + tool calling).
   Works with OpenAI, DeepSeek, OpenRouter, Ollama, LM Studio, Groq, Mistral, etc.
4. **Branding swap** — `opencode`→`opentitam`, `OpenCode`→`OpenTitam`,
   `~/.config/opencode`→`~/.config/opentitam`, env prefix `OPENCODE_*`→`OPENTITAM_*`.
5. **Local backend** — sessions/config/approvals stored under `~/.config/opentitam/`
   and `%LOCALAPPDATA%\opentitam\`.

## File plan (build order)
```
opentitam/
├── go.mod
├── PLAN.md                        this file
├── cmd/opentitam/main.go          entry point
├── internal/cli/                  root, run, models, setup, version subcommands
├── internal/config/               global+project config load/merge, {env:}/{file:} substitution
├── internal/provider/             registry, dynamic GET /models, SSE chat client w/ tool calls
├── internal/permissions/          action/resource/effect engine, wildcards, last-match-wins
├── internal/tools/                read, write, edit, glob, grep, shell + JSON schemas
├── internal/agent/                system prompt, agent loop, context trim
├── internal/session/              local JSON session store (list/resume)
├── internal/slash/                /models /sessions /new /compact /help /exit
├── internal/tui/                  OpenCode-style TUI (spec: docs/tui-spec.md)
├── docs/tui-spec.md               researched TUI design spec
├── mockserver/                    fake OpenAI server for integration tests
└── *_test.go                      unit + integration tests
```

## Features
- `opentitam` → TUI · `opentitam run "prompt"` → headless · `opentitam setup` → wizard
  (baseURL + apiKey → fetch models → pick default) · `opentitam models` · `--version`
- Dynamic model list only from the provider's `/models` endpoint (no hardcoded catalog)
- 6 coding tools: read, write, edit, glob, grep, shell (PowerShell-aware)
- OpenCode-style permission engine: allow/ask/deny, `*`/`?` wildcards, last-match-wins,
  `.env` ask-by-default, approval prompt (Allow once / Allow always / Reject)
- Streaming replies (SSE) + tool-call agent loop
- Sessions stored locally, listable/resumable
- Context auto-trim by estimated tokens

## Test plan
1. Unit tests: config merge, permission matching, every tool, model fetch parsing
2. Mock OpenAI server on localhost: `/models` + `/chat/completions` (SSE + tool calls)
   → full `opentitam run` integration round-trip
3. `go vet` + `go test ./...` + `go build`
4. Smoke: `--help`, `models`, mock-server chat round-trip

## Skills note
Only the `opencode` skill is relevant to this build and it is already loaded
(config/tools/permissions behavior source of truth). Other available skills
(cloudflare, sandbox, wrangler, web-perf…) are unrelated to this Go CLI.
