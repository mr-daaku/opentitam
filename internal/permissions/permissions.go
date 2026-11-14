// Package permissions implements the OpenTitam permission engine.
//
// Rules are {action, resource, effect} triples with simple whole-value
// wildcards (* and ?). Rules are evaluated in order; the LAST matching rule
// wins. If nothing matches, the effect is "ask".
package permissions

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mr-daaku/opentitam/internal/config"
)

// Engine evaluates permission rules.
type Engine struct {
	rules []config.Permission
}

// BasePolicy is the ordered default policy every session starts with
// (mirrors OpenCode's base policy).
var BasePolicy = []config.Permission{
	{Action: "*", Resource: "*", Effect: "allow"},
	{Action: "read", Resource: "*.env", Effect: "ask"},
	{Action: "read", Resource: "*.env.*", Effect: "ask"},
	{Action: "read", Resource: "*.env.example", Effect: "allow"},
}

// New builds an engine from base policy + user rules (user rules appended last).
func New(userRules []config.Permission) *Engine {
	rules := make([]config.Permission, 0, len(BasePolicy)+len(userRules))
	rules = append(rules, BasePolicy...)
	rules = append(rules, userRules...)
	return &Engine{rules: rules}
}

// normalizePath converts backslashes to slashes; on Windows matching is
// case-insensitive (handled in match).
func normalizePath(p string) string {
	return strings.ReplaceAll(p, "/", string(filepath.Separator))
}

// match reports whether pattern matches the whole value.
// Supports * (zero or more chars, including /) and ? (exactly one char).
func match(pattern, value string) bool {
	// Case-insensitive on Windows (and always for drive letters safety).
	if runtime.GOOS == "windows" {
		pattern = strings.ToLower(pattern)
		value = strings.ToLower(value)
	}
	return globMatch(pattern, value)
}

func globMatch(pattern, value string) bool {
	// Iterative glob matching with backtracking.
	pi, vi := 0, 0
	starP, starV := -1, -1
	for vi < len(value) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == value[vi]) {
			pi++
			vi++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starP = pi
			starV = vi
			pi++
		} else if starP != -1 {
			pi = starP + 1
			starV++
			vi = starV
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// Check evaluates all rules for one action+resource pair.
// Returns "allow", "ask", or "deny".
func (e *Engine) Check(action, resource string) string {
	effect := "" // no match → default ask
	for _, r := range e.rules {
		if r.Action != "*" && r.Action != action {
			continue
		}
		if !matchResource(r.Resource, resource) {
			continue
		}
		effect = r.Effect
	}
	if effect == "" {
		return "ask"
	}
	return effect
}

// matchResource matches a rule resource against the actual resource.
// A shell pattern ending in " *" also matches the command without arguments.
func matchResource(pattern, resource string) bool {
	if match(pattern, resource) {
		return true
	}
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		if runtime.GOOS == "windows" {
			prefix = strings.ToLower(prefix)
			resource = strings.ToLower(resource)
		}
		if resource == prefix {
			return true
		}
		if strings.HasPrefix(resource, prefix+" ") {
			return true
		}
	}
	return false
}

// CheckMany evaluates several resources for one operation:
// any deny → deny; else any ask → ask; else allow.
func (e *Engine) CheckMany(action string, resources []string) string {
	ask := false
	for _, res := range resources {
		switch e.Check(action, res) {
		case "deny":
			return "deny"
		case "ask":
			ask = true
		}
	}
	if ask {
		return "ask"
	}
	return "allow"
}
