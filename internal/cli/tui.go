package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/tui"
)

// runTUI launches the interactive terminal UI.
// Args: optional directory (opencode-style: `opentitam <directory>`).
func runTUI(args []string) int {
	dir := cwd()
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		abs, err := filepath.Abs(args[0])
		if err == nil {
			if st, err := os.Stat(abs); err == nil && st.IsDir() {
				dir = abs
				if err := os.Chdir(dir); err != nil {
					fmt.Fprintln(os.Stderr, "cannot enter directory:", err)
					return 1
				}
			}
		}
	}

	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if len(cfg.Providers) == 0 {
		fmt.Fprintln(os.Stderr, "No providers configured. Run `opentitam setup` first.")
		return 1
	}
	if err := tui.Run(cfg, dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
