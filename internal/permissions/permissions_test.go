package permissions

import (
	"testing"

	"github.com/mr-daaku/opentitam/internal/config"
)

func TestBasePolicyDefaults(t *testing.T) {
	e := New(nil)
	if got := e.Check("read", "src/main.go"); got != "allow" {
		t.Errorf("read main.go = %s, want allow", got)
	}
	if got := e.Check("read", ".env"); got != "ask" {
		t.Errorf("read .env = %s, want ask", got)
	}
	if got := e.Check("read", ".env.local"); got != "ask" {
		t.Errorf("read .env.local = %s, want ask", got)
	}
	if got := e.Check("read", ".env.example"); got != "allow" {
		t.Errorf("read .env.example = %s, want allow", got)
	}
}

func TestLastMatchWins(t *testing.T) {
	e := New([]config.Permission{
		{Action: "shell", Resource: "*", Effect: "ask"},
		{Action: "shell", Resource: "git status *", Effect: "allow"},
		{Action: "shell", Resource: "git push *", Effect: "deny"},
	})
	cases := map[string]string{
		"git status":           "allow", // prefix rule matches bare command too
		"git status --short":   "allow",
		"git push origin main": "deny",
		"rm -rf /":             "ask",
	}
	for cmd, want := range cases {
		if got := e.Check("shell", cmd); got != want {
			t.Errorf("shell %q = %s, want %s", cmd, got, want)
		}
	}
}

func TestWildcards(t *testing.T) {
	// Official semantics: * = zero or more chars INCLUDING / (whole-value anchored).
	if !match("packages/docs/*.mdx", "packages/docs/guide.mdx") {
		t.Errorf("single-star segment should match")
	}
	if !match("packages/docs/*.mdx", "packages/docs/sub/guide.mdx") {
		t.Errorf("* should cross / per permission docs")
	}
	// Whole-value anchoring: pattern must match the entire string.
	if match("read", "readme.txt") {
		t.Errorf("pattern must anchor whole value")
	}
	// ? matches exactly one char.
	if !match("file?.txt", "fileA.txt") {
		t.Errorf("? should match one char")
	}
	if match("file?.txt", "fileAB.txt") {
		t.Errorf("? should not match 2 chars")
	}
	// Rule resource with trailing " *" prefix semantics.
	if !matchResource("git status *", "git status") {
		t.Errorf("prefix rule should match bare command")
	}
	if !matchResource("git status *", "git status --short") {
		t.Errorf("prefix rule should match with args")
	}
	if matchResource("git status *", "git push") {
		t.Errorf("prefix rule must not match other commands")
	}
}

func TestCheckMany(t *testing.T) {
	e := New([]config.Permission{
		{Action: "edit", Resource: "ok.txt", Effect: "allow"},
		{Action: "edit", Resource: "bad.txt", Effect: "deny"},
	})
	if got := e.CheckMany("edit", []string{"ok.txt", "other.txt"}); got != "allow" {
		t.Errorf("got %s", got)
	}
	if got := e.CheckMany("edit", []string{"ok.txt", "bad.txt"}); got != "deny" {
		t.Errorf("deny must win: got %s", got)
	}
	// Unknown resource → ask (no rule matches except base */* allow… base has
	// action * resource * allow, so unknown is actually allow via base).
	if got := e.CheckMany("edit", []string{"ok.txt", "unknown.txt"}); got != "allow" {
		t.Errorf("got %s", got)
	}
}

func TestNoMatchIsAsk(t *testing.T) {
	// Engine with no base and no matching rules → ask.
	e := &Engine{rules: nil}
	if got := e.Check("shell", "anything"); got != "ask" {
		t.Errorf("got %s, want ask", got)
	}
}
