package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

// writeTool creates or completely replaces a text file.
func writeTool() *Tool {
	return &Tool{
		Name:        "write",
		Description: "Create a new file or completely overwrite an existing file with the given content. Creates missing parent directories. Use edit instead to change part of an existing file.",
		Schema: funcSchema("", map[string]any{
			"path":    strProp("File path (absolute or relative to cwd)"),
			"content": strProp("Full file content"),
		}, []string{"path", "content"}),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", toolErrf("invalid arguments: %v", err)
			}
			if in.Path == "" {
				return "", toolErrf("path is required")
			}
			abs, err := filepath.Abs(in.Path)
			if err != nil {
				return "", toolErrf("%v", err)
			}
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return "", toolErrf("cannot create parent dir: %v", err)
			}
			// Preserve file mode if it exists.
			mode := os.FileMode(0o644)
			if st, err := os.Stat(abs); err == nil {
				mode = st.Mode().Perm()
			}
			if err := os.WriteFile(abs, []byte(in.Content), mode); err != nil {
				return "", toolErrf("cannot write %s: %v", in.Path, err)
			}
			return "Wrote " + itoa(len(in.Content)) + " bytes to " + in.Path, nil
		},
	}
}

func itoa(n int) string {
	return fmtInt(n)
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
