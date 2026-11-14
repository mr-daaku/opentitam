package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// grepTool searches file contents and returns paths, line numbers, previews.
func grepTool() *Tool {
	return &Tool{
		Name:        "grep",
		Description: "Search file contents with a regular expression (or literal text). Returns matching file paths, line numbers, and line previews. Narrow with path, include (glob), caseSensitive, and limit.",
		Schema: funcSchema("", map[string]any{
			"pattern":      strProp("Regular expression (set literal=true for exact text)"),
			"path":         strProp("Directory or file to search (default: cwd)"),
			"include":      strProp("Only search files matching this glob (e.g. *.go)"),
			"literal":      map[string]any{"type": "boolean", "description": "Treat pattern as literal text, not regex (default false)"},
			"caseSensitive": map[string]any{"type": "boolean", "description": "Case-sensitive matching (default true)"},
			"limit":        map[string]any{"type": "integer", "description": "Max matching lines (default 100)"},
		}, []string{"pattern"}),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Pattern       string `json:"pattern"`
				Path          string `json:"path"`
				Include       string `json:"include"`
				Literal       bool   `json:"literal"`
				CaseSensitive bool   `json:"caseSensitive"`
				Limit         int    `json:"limit"`
			}
			// Default CaseSensitive true: JSON false is indistinguishable from
			// absent, so use a pointer via raw decode fallback.
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(args, &raw)
			if err := json.Unmarshal(args, &in); err != nil {
				return "", toolErrf("invalid arguments: %v", err)
			}
			if v, ok := raw["caseSensitive"]; ok {
				var b bool
				if json.Unmarshal(v, &b) == nil {
					in.CaseSensitive = b
				}
			} else {
				in.CaseSensitive = true
			}
			if in.Pattern == "" {
				return "", toolErrf("pattern is required")
			}
			if in.Limit <= 0 {
				in.Limit = 100
			}
			pattern := in.Pattern
			if in.Literal {
				pattern = regexp.QuoteMeta(pattern)
			}
			if !in.CaseSensitive {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return "", toolErrf("invalid regex: %v", err)
			}
			root := "."
			if in.Path != "" {
				root = in.Path
			}
			abs, err := filepath.Abs(root)
			if err != nil {
				return "", toolErrf("%v", err)
			}
			st, err := os.Stat(abs)
			if err != nil {
				return "", toolErrf("cannot access %s: %v", root, err)
			}
			var files []string
			if st.IsDir() {
				filepath.WalkDir(abs, func(p string, d os.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					name := d.Name()
					if d.IsDir() {
						if !in.CaseSensitive && false {
							return nil
						}
						if name == "node_modules" || name == ".git" || name == "vendor" || name == "dist" {
							return filepath.SkipDir
						}
						if strings.HasPrefix(name, ".") && p != abs {
							return filepath.SkipDir
						}
						return nil
					}
					if strings.HasPrefix(name, ".") {
						return nil
					}
					if in.Include != "" {
						if ok, _ := filepath.Match(in.Include, name); !ok {
							return nil
						}
					}
					files = append(files, p)
					return nil
				})
			} else {
				files = []string{abs}
			}
			sort.Strings(files)

			var b strings.Builder
			total := 0
			matchedFiles := 0
			for _, f := range files {
				if total >= in.Limit {
					break
				}
				lines, ok := grepFile(f, re, in.Limit-total)
				if !ok {
					continue
				}
				matchedFiles++
				rel, err := filepath.Rel(abs, f)
				if err != nil || st.IsDir() == false {
					rel = f
				}
				if st.IsDir() {
					if r2, e2 := filepath.Rel(abs, f); e2 == nil {
						rel = filepath.ToSlash(r2)
					}
				}
				for _, ln := range lines {
					fmt.Fprintf(&b, "%s:%d: %s\n", rel, ln.no, ln.text)
					total++
				}
			}
			if total == 0 {
				return fmt.Sprintf("No matches for %q in %s", in.Pattern, root), nil
			}
			summary := fmt.Sprintf("%d match(es) in %d file(s):\n", total, matchedFiles)
			return summary + b.String(), nil
		},
	}
}

type grepLine struct {
	no   int
	text string
}

func grepFile(path string, re *regexp.Regexp, max int) ([]grepLine, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	// Skip obviously binary files quickly.
	if st, err := f.Stat(); err == nil && st.Size() > 2*1024*1024 {
		return nil, false
	}
	var out []grepLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	no := 0
	for sc.Scan() {
		no++
		line := sc.Text()
		if re.MatchString(line) {
			if len(line) > 300 {
				line = line[:300] + "…"
			}
			out = append(out, grepLine{no: no, text: strings.TrimSpace(line)})
			if len(out) >= max {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
