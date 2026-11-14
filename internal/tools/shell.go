package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// shellTool runs a command in the host shell (PowerShell on Windows).
func shellTool(shellPath string) *Tool {
	return &Tool{
		Name:        "shell",
		Description: "Execute a shell command and return its stdout/stderr. Prefer the workdir parameter over cd. On Windows the shell is PowerShell. Commands have a default timeout of 120 seconds.",
		Schema: funcSchema("", map[string]any{
			"command": strProp("The command line to execute"),
			"workdir": strProp("Working directory (default: cwd)"),
			"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 120, max 600)"},
		}, []string{"command"}),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Command string `json:"command"`
				Workdir string `json:"workdir"`
				Timeout int    `json:"timeout"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", toolErrf("invalid arguments: %v", err)
			}
			if strings.TrimSpace(in.Command) == "" {
				return "", toolErrf("command is required")
			}
			if in.Timeout <= 0 {
				in.Timeout = 120
			}
			if in.Timeout > 600 {
				in.Timeout = 600
			}
			workdir := in.Workdir
			if workdir == "" {
				workdir, _ = os.Getwd()
			}
			if abs, err := filepath.Abs(workdir); err == nil {
				workdir = abs
			}
			cctx, cancel := context.WithTimeout(ctx, time.Duration(in.Timeout)*time.Second)
			defer cancel()

			var cmd *exec.Cmd
			shell := shellPath
			if shell == "" {
				if runtime.GOOS == "windows" {
					shell = "powershell"
				} else {
					shell = "bash"
				}
			}
			base := filepath.Base(shell)
			if strings.Contains(strings.ToLower(base), "powershell") || strings.Contains(strings.ToLower(base), "pwsh") {
				cmd = exec.CommandContext(cctx, shell, "-NoProfile", "-NonInteractive", "-Command", in.Command)
			} else if strings.Contains(base, "cmd") {
				cmd = exec.CommandContext(cctx, shell, "/C", in.Command)
			} else {
				cmd = exec.CommandContext(cctx, shell, "-c", in.Command)
			}
			cmd.Dir = workdir
			cmd.Env = append(os.Environ(), "OPENTITAM=1")

			var out, errBuf bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errBuf

			runErr := cmd.Run()

			stdout := out.String()
			stderr := errBuf.String()
			combined := stdout
			if strings.TrimSpace(stderr) != "" {
				if combined != "" && !strings.HasSuffix(combined, "\n") {
					combined += "\n"
				}
				combined += "--- stderr ---\n" + stderr
			}
			combined = truncateOutput(combined, 30000)

			if cctx.Err() == context.DeadlineExceeded {
				return combined + "\n[command timed out]", toolErrf("command timed out after %ds", in.Timeout)
			}
			if runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					return combined, fmt.Errorf("exit code %d", exitErr.ExitCode())
				}
				return combined, toolErrf("failed to run %s: %v", shell, runErr)
			}
			if combined == "" {
				return "(no output)", nil
			}
			return combined, nil
		},
	}
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	head := s[:max/2]
	tail := s[len(s)-max/2:]
	return head + fmt.Sprintf("\n… [%d bytes truncated] …\n", len(s)-max) + tail
}
