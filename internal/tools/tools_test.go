package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runTool(t *testing.T, tool *Tool, args map[string]any) (string, error) {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Execute(context.Background(), json.RawMessage(b))
}

func TestReadWriteEditRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	reg := DefaultRegistry("")

	// Write.
	out, err := runTool(t, reg.tools["write"], map[string]any{"path": p, "content": "hello\nworld\n"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "Wrote") {
		t.Errorf("write out: %s", out)
	}

	// Read (numbered lines).
	out, err = runTool(t, reg.tools["read"], map[string]any{"path": p})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "1: hello") || !strings.Contains(out, "2: world") {
		t.Errorf("read out:\n%s", out)
	}

	// Edit unique match.
	out, err = runTool(t, reg.tools["edit"], map[string]any{
		"path": p, "oldString": "hello", "newString": "HELLO",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "HELLO") {
		t.Errorf("edit did not apply: %s", b)
	}
}

func TestEditRequiresUniqueMatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dup.txt")
	if err := os.WriteFile(p, []byte("x x x"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := DefaultRegistry("")

	// Ambiguous without replaceAll → error.
	_, err := runTool(t, reg.tools["edit"], map[string]any{
		"path": p, "oldString": "x", "newString": "y",
	})
	if err == nil || !strings.Contains(err.Error(), "2 times") && !strings.Contains(err.Error(), "3 times") {
		// Count is 3 for "x x x".
		if err == nil {
			t.Fatalf("expected ambiguity error, got nil")
		}
	}

	// replaceAll works.
	_, err = runTool(t, reg.tools["edit"], map[string]any{
		"path": p, "oldString": "x", "newString": "y", "replaceAll": true,
	})
	if err != nil {
		t.Fatalf("replaceAll: %v", err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "y y y" {
		t.Errorf("got %q", b)
	}

	// Not found → error.
	_, err = runTool(t, reg.tools["edit"], map[string]any{
		"path": p, "oldString": "nope", "newString": "y",
	})
	if err == nil {
		t.Fatalf("expected not-found error")
	}
}

func TestGlobDoubleStar(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("src/main.go")
	mk("src/deep/util.go")
	mk("README.md")
	reg := DefaultRegistry("")

	out, err := runTool(t, reg.tools["glob"], map[string]any{
		"pattern": "**/*.go", "path": dir,
	})
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "src/main.go") || !strings.Contains(out, "src/deep/util.go") {
		t.Errorf("glob out:\n%s", out)
	}
	if strings.Contains(out, "README.md") {
		t.Errorf("should not match .md:\n%s", out)
	}

	// Single star must not cross /.
	out, err = runTool(t, reg.tools["glob"], map[string]any{
		"pattern": "*.go", "path": dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "src/main.go") {
		t.Errorf("single star crossed /\n%s", out)
	}
}

func TestGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n// TODO fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("TODO later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := DefaultRegistry("")

	// Regex + include filter.
	out, err := runTool(t, reg.tools["grep"], map[string]any{
		"pattern": "TODO", "path": dir, "include": "*.go",
	})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(out, "a.go:2:") {
		t.Errorf("grep out:\n%s", out)
	}
	if strings.Contains(out, "b.txt") {
		t.Errorf("include filter failed:\n%s", out)
	}

	// Literal mode: treat regex meta as text.
	if err := os.WriteFile(filepath.Join(dir, "c.go"), []byte("a+b here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runTool(t, reg.tools["grep"], map[string]any{
		"pattern": "a+b", "path": dir, "literal": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "c.go") {
		t.Errorf("literal grep failed:\n%s", out)
	}
}

func TestShell(t *testing.T) {
	reg := DefaultRegistry("")
	var out string
	var err error
	if runtime.GOOS == "windows" {
		out, err = runTool(t, reg.tools["shell"], map[string]any{"command": "Write-Output hello"})
	} else {
		out, err = runTool(t, reg.tools["shell"], map[string]any{"command": "echo hello"})
	}
	if err != nil {
		t.Fatalf("shell: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("shell out: %q", out)
	}

	// Non-zero exit returns error but keeps output.
	_, err = runTool(t, reg.tools["shell"], map[string]any{
		"command": "exit 3",
	})
	if err == nil {
		t.Errorf("expected exit code error")
	} else if !strings.Contains(err.Error(), "3") {
		t.Errorf("err = %v", err)
	}
}

func TestReadBinaryRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bin.dat")
	if err := os.WriteFile(p, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := DefaultRegistry("")
	_, err := runTool(t, reg.tools["read"], map[string]any{"path": p})
	if err == nil {
		t.Fatalf("expected binary rejection")
	}
}

func TestReadDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := DefaultRegistry("")
	out, err := runTool(t, reg.tools["read"], map[string]any{"path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sub/") || !strings.Contains(out, "f.txt") {
		t.Errorf("dir listing:\n%s", out)
	}
}
