package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
)

func newTestModel() *Model {
	return &Model{modelID: "test-model", vp: viewport.New(80, 20)}
}

// Raw markdown must never reach the screen: no ** left over.
func TestRenderMarkdownStripsRawMarkers(t *testing.T) {
	in := "# Title\n\nSome **bold** and *italic* and `code` here.\n\n- item one\n- item two\n\n```go\nfmt.Println(\"hi\")\n```"
	out := renderMarkdown(in)
	if strings.Contains(out, "**") {
		t.Errorf("raw ** leaked into output:\n%s", out)
	}
	for _, want := range []string{"Title", "bold", "italic", "code", "item one", "fmt.Println"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Fenced block content survives as its own line.
	if !strings.Contains(out, "fmt.Println") {
		t.Errorf("code block content lost:\n%s", out)
	}
}

func TestRenderMarkdownUnclosedFenceDuringStream(t *testing.T) {
	out := renderMarkdown("text\n```go\npartial")
	if !strings.Contains(out, "partial") {
		t.Errorf("partial code fence lost: %q", out)
	}
}

func TestRenderMarkdownEmpty(t *testing.T) {
	if got := renderMarkdown(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// The palette filter must match bare names with and without a slash.
func TestFilterCommands(t *testing.T) {
	if got := filterCommands(""); len(got) != len(allCommands()) {
		t.Errorf("empty filter should list all commands, got %d", len(got))
	}
	for _, q := range []string{"/mo", "mo", "/models"} {
		got := filterCommands(q)
		ok := false
		for _, c := range got {
			if c == "/models" {
				ok = true
			}
		}
		if !ok {
			t.Errorf("filter %q should match /models, got %v", q, got)
		}
	}
	if got := filterCommands("zzz"); len(got) != 0 {
		t.Errorf("no match expected, got %v", got)
	}
}

// A text_done must CONVERT the streaming block, never append a second copy —
// that was the "reply shows twice" bug.
func TestTextDoneConvertsStreamingBlock(t *testing.T) {
	m := newTestModel()
	m.blocks = []block{
		{kind: "user", body: "hi"},
		{kind: "streaming", label: "assistant", body: "hello **world**"},
	}
	m.streamBuf.WriteString("hello **world**")
	m.turnStart = time.Now()

	m.Update(textDoneMsg{})
	assistants := 0
	for _, b := range m.blocks {
		if b.kind == "assistant" {
			assistants++
		}
	}
	if assistants != 1 {
		t.Fatalf("expected exactly 1 assistant block, got %d: %+v", assistants, m.blocks)
	}
	if m.blocks[len(m.blocks)-1].kind == "streaming" {
		t.Fatalf("streaming block left behind: %+v", m.blocks)
	}
	if !strings.Contains(m.blocks[1].body, "hello") {
		t.Errorf("assistant body lost: %+v", m.blocks[1])
	}
}

// Empty stream must drop the placeholder (no blank reply).
func TestTextDoneDropsEmptyStream(t *testing.T) {
	m := newTestModel()
	m.blocks = []block{{kind: "streaming", label: "assistant", body: ""}}
	m.turnStart = time.Now()
	m.Update(textDoneMsg{})
	for _, b := range m.blocks {
		if b.kind == "streaming" || b.kind == "assistant" {
			t.Fatalf("empty block should be gone: %+v", m.blocks)
		}
	}
}

// Thinking starts hidden (collapsed) and toggles on click.
func TestThinkingHiddenByDefaultAndToggles(t *testing.T) {
	m := newTestModel()
	m.blocks = []block{{kind: "thinking", body: "step 1\nstep 2"}}
	m.renderTranscript()
	if m.blocks[0].expanded {
		t.Fatal("thinking must start collapsed")
	}
	if len(m.blockOfLine) == 0 {
		t.Fatal("transcript produced no lines")
	}
	// Simulate a click on the collapsed line.
	line := 0
	bi := m.blockOfLine[line]
	if bi != 0 || m.blocks[bi].kind != "thinking" {
		t.Fatalf("hit-test mapping wrong: line0 -> block %d (%s)", bi, m.blocks[bi].kind)
	}
	m.blocks[bi].expanded = true
	m.renderTranscript()
	// Expanded view must contain the body text.
	joined := m.vp.View()
	if !strings.Contains(joined, "step 1") {
		t.Errorf("expanded thinking should show body, got:\n%s", joined)
	}
}
