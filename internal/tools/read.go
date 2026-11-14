package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// readTool returns a file or directory listing with numbered lines.
func readTool() *Tool {
	return &Tool{
		Name:        "read",
		Description: "Read a text file or list a directory. Returns numbered lines. Use offset/limit to page large files.",
		Schema: funcSchema("", map[string]any{
			"path":   strProp("File or directory path (absolute or relative to cwd)"),
			"offset": map[string]any{"type": "integer", "description": "1-based line number to start from (default 1)"},
			"limit":  map[string]any{"type": "integer", "description": "Max lines to return (default 2000)"},
		}, []string{"path"}),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", toolErrf("invalid arguments: %v", err)
			}
			if in.Path == "" {
				return "", toolErrf("path is required")
			}
			if in.Offset <= 0 {
				in.Offset = 1
			}
			if in.Limit <= 0 {
				in.Limit = 2000
			}
			abs, err := filepath.Abs(in.Path)
			if err != nil {
				return "", toolErrf("%v", err)
			}
			st, err := os.Stat(abs)
			if err != nil {
				return "", toolErrf("cannot read %s: %v", in.Path, err)
			}
			if st.IsDir() {
				return readDir(abs)
			}
			return readFile(abs, in.Offset, in.Limit)
		},
	}
}

func readDir(abs string) (string, error) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", toolErrf("cannot list %s: %v", abs, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Directory: %s\n", abs)
	if len(entries) == 0 {
		b.WriteString("(empty)\n")
		return b.String(), nil
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		fmt.Fprintf(&b, "%s\n", name)
	}
	return b.String(), nil
}

const (
	maxReadLines   = 2000
	maxLineChars   = 2000
	maxReadBytes   = 50 * 1024 // per page
	maxBinaryBytes = 20 * 1024 * 1024
)

func readFile(abs string, offset, limit int) (string, error) {
	st, err := os.Stat(abs)
	if err != nil {
		return "", toolErrf("%v", err)
	}
	if st.Size() > maxBinaryBytes {
		return "", toolErrf("file too large (%d bytes)", st.Size())
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", toolErrf("cannot read %s: %v", abs, err)
	}
	// Binary detection: NUL byte or invalid UTF-8 in first chunk.
	if len(data) > 0 {
		chunk := data
		if len(chunk) > 8192 {
			chunk = chunk[:8192]
		}
		if strings.ContainsRune(string(chunk), 0) || !utf8.Valid(chunk) {
			return "", toolErrf("%s appears to be a binary file", abs)
		}
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	// Drop trailing empty line from final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := offset - 1
	if start > len(lines) {
		return fmt.Sprintf("File %s (offset %d beyond EOF, %d lines total)\n", abs, offset, len(lines)), nil
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	if end-start > maxReadLines {
		end = start + maxReadLines
	}
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s (%d lines)\n", abs, len(lines))
	bytes := 0
	for i := start; i < end; i++ {
		line := lines[i]
		if utf8.RuneCountInString(line) > maxLineChars {
			runes := []rune(line)
			line = string(runes[:maxLineChars]) + "… [line truncated]"
		}
		out := fmt.Sprintf("%6d: %s\n", i+1, line)
		bytes += len(out)
		if bytes > maxReadBytes {
			b.WriteString("… [page byte limit reached]\n")
			break
		}
		b.WriteString(out)
	}
	return b.String(), nil
}
