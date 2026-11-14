package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// globTool finds file paths matching a pattern (e.g. **/*.ts).
func globTool() *Tool {
	return &Tool{
		Name:        "glob",
		Description: "Find file paths matching a glob pattern like **/*.ts or src/**/*.go. Searches the current directory unless path is given. Results are sorted; use limit to cap output.",
		Schema: funcSchema("", map[string]any{
			"pattern": strProp("Glob pattern (e.g. **/*.ts, internal/**/*.go)"),
			"path":    strProp("Directory to search (default: cwd)"),
			"hidden":  map[string]any{"type": "boolean", "description": "Include hidden files/dirs (default false)"},
			"limit":   map[string]any{"type": "integer", "description": "Max results (default 100)"},
		}, []string{"pattern"}),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
				Hidden  bool   `json:"hidden"`
				Limit   int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", toolErrf("invalid arguments: %v", err)
			}
			if in.Pattern == "" {
				return "", toolErrf("pattern is required")
			}
			if in.Limit <= 0 {
				in.Limit = 100
			}
			root := "."
			if in.Path != "" {
				root = in.Path
			}
			abs, err := filepath.Abs(root)
			if err != nil {
				return "", toolErrf("%v", err)
			}
			if st, err := os.Stat(abs); err != nil || !st.IsDir() {
				return "", toolErrf("not a directory: %s", root)
			}
			var matches []string
			walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // skip unreadable
				}
				name := d.Name()
				if !in.Hidden && strings.HasPrefix(name, ".") && p != abs {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				// Skip heavy dirs.
				if d.IsDir() && (name == "node_modules" || name == ".git" || name == "vendor") {
					return filepath.SkipDir
				}
				if d.IsDir() {
					return nil
				}
				rel, rerr := filepath.Rel(abs, p)
				if rerr != nil {
					return nil
				}
				rel = filepath.ToSlash(rel)
				// Use the segment-based matcher (filepath.Match treats '/' as a
				// non-separator on Windows, so it wrongly matches across dirs).
				if simplePathMatch(in.Pattern, rel) {
					matches = append(matches, rel)
				}
				return nil
			})
			if walkErr != nil {
				return "", toolErrf("walk error: %v", walkErr)
			}
			sort.Strings(matches)
			if len(matches) == 0 {
				return fmt.Sprintf("No files match %q under %s", in.Pattern, root), nil
			}
			truncated := false
			if len(matches) > in.Limit {
				matches = matches[:in.Limit]
				truncated = true
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d match(es) for %q under %s:\n", len(matches), in.Pattern, root)
			for _, m := range matches {
				b.WriteString(m + "\n")
			}
			if truncated {
				b.WriteString("… [truncated at limit]\n")
			}
			return b.String(), nil
		},
	}
}

// matchDoubleStar matches patterns containing ** against slash paths.
// Supports forms: "**/x", "a/**/b", "a/**", "**/*.ext".
func matchDoubleStar(pattern, path string) bool {
	parts := strings.Split(pattern, "**")
	// Simple cases only; fall back to segment matching.
	if len(parts) == 2 {
		prefix := strings.TrimSuffix(parts[0], "/")
		suffix := strings.TrimPrefix(parts[1], "/")
		if prefix != "" && !strings.HasPrefix(path, prefix+"/") && path != prefix {
			return false
		}
		rest := path
		if prefix != "" {
			rest = strings.TrimPrefix(path, prefix+"/")
			if rest == path && path != prefix {
				return false
			}
		}
		if suffix == "" {
			return true
		}
		if strings.HasPrefix(suffix, "*.") {
			// **/*.ext → any depth, file ends with .ext
			return strings.HasSuffix(rest, strings.TrimPrefix(suffix, "*"))
		}
		// suffix may itself contain single-star segments
		segs := strings.Split(suffix, "/")
		restSegs := strings.Split(rest, "/")
		return matchSegments(segs, restSegs)
	}
	// Generic fallback: exact after removing ** segments is too ambiguous; try
	// converting whole pattern to a path match by treating ** as ".*".
	return simplePathMatch(pattern, path)
}

func matchSegments(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if len(segs) == 0 {
		return false
	}
	if ok, _ := filepath.Match(pat[0], segs[0]); ok || pat[0] == "**" {
		if pat[0] == "**" {
			// Try consuming zero or more segments.
			for i := 0; i <= len(segs); i++ {
				if matchSegments(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		return matchSegments(pat[1:], segs[1:])
	}
	return false
}

func simplePathMatch(pattern, path string) bool {
	// Convert glob to segment match with ** support.
	patSegs := strings.Split(filepath.ToSlash(pattern), "/")
	pathSegs := strings.Split(filepath.ToSlash(path), "/")
	return matchSegments(patSegs, pathSegs)
}
