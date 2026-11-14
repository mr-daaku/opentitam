package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// editTool replaces text in an existing file. The old text must match
// exactly once unless replaceAll is true.
func editTool() *Tool {
	return &Tool{
		Name:        "edit",
		Description: "Replace text in an existing file. oldString must match exactly once (include enough surrounding context to be unique); set replaceAll to replace every occurrence. newString must differ from oldString.",
		Schema: funcSchema("", map[string]any{
			"path":       strProp("File path (absolute or relative to cwd)"),
			"oldString":  strProp("Exact text to find (must be unique in the file unless replaceAll)"),
			"newString":  strProp("Replacement text"),
			"replaceAll": map[string]any{"type": "boolean", "description": "Replace every occurrence instead of requiring uniqueness (default false)"},
		}, []string{"path", "oldString", "newString"}),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Path       string `json:"path"`
				OldString  string `json:"oldString"`
				NewString  string `json:"newString"`
				ReplaceAll bool   `json:"replaceAll"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", toolErrf("invalid arguments: %v", err)
			}
			if in.Path == "" || in.OldString == "" {
				return "", toolErrf("path and oldString are required")
			}
			if in.OldString == in.NewString {
				return "", toolErrf("newString must differ from oldString")
			}
			abs, err := filepath.Abs(in.Path)
			if err != nil {
				return "", toolErrf("%v", err)
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return "", toolErrf("cannot read %s: %v", in.Path, err)
			}
			content := string(data)
			count := strings.Count(content, in.OldString)
			if count == 0 {
				return "", toolErrf("oldString not found in %s (exact match required — check whitespace and indentation)", in.Path)
			}
			if count > 1 && !in.ReplaceAll {
				return "", toolErrf("oldString matches %d times in %s — add surrounding context to make it unique, or set replaceAll=true", count, in.Path)
			}
			var result string
			if in.ReplaceAll {
				result = strings.ReplaceAll(content, in.OldString, in.NewString)
			} else {
				result = strings.Replace(content, in.OldString, in.NewString, 1)
			}
			mode := os.FileMode(0o644)
			if st, err := os.Stat(abs); err == nil {
				mode = st.Mode().Perm()
			}
			if err := os.WriteFile(abs, []byte(result), mode); err != nil {
				return "", toolErrf("cannot write %s: %v", in.Path, err)
			}
			n := count
			if !in.ReplaceAll {
				n = 1
			}
			return fmt.Sprintf("Edited %s (%d replacement%s)", in.Path, n, plural(n)), nil
		},
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
