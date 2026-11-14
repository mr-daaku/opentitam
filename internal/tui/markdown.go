package tui

import (
	"regexp"
	"strings"
)

// renderMarkdown turns assistant markdown into colored terminal text so the
// chat never shows raw **, ` or # characters. Supported: fenced code blocks,
// inline code, bold, italic, headings, lists, blockquotes, links, rules.
func renderMarkdown(src string) string {
	if src == "" {
		return src
	}
	lines := strings.Split(src, "\n")
	var out []string
	var code []string
	inCode := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCode {
				out = append(out, renderCodeBlock(code))
				code = nil
				inCode = false
			} else {
				inCode = true
			}
			continue
		}
		if inCode {
			code = append(code, line)
			continue
		}
		out = append(out, renderMDLine(line))
	}
	if inCode {
		// Unclosed fence (still streaming): show what we have.
		out = append(out, renderCodeBlock(code))
	}
	return strings.Join(out, "\n")
}

func renderCodeBlock(code []string) string {
	body := strings.Join(code, "\n")
	if body == "" {
		body = " "
	}
	return styleMDCodeBl.Render(body)
}

func renderMDLine(line string) string {
	// Horizontal rule.
	if regexpHR.MatchString(line) {
		return styleMuted.Render(strings.Repeat("─", 40))
	}
	// Headings.
	if m := regexpHeading.FindStringSubmatch(line); m != nil {
		text := renderMDInline(m[2])
		switch len(m[1]) {
		case 1:
			return styleMDH1.Render("▸ ") + styleMDH1.Render(text)
		case 2:
			return styleMDH2.Render("▸ ") + styleMDH2.Render(text)
		default:
			return styleMDH3.Render("▸ ") + styleMDH3.Render(text)
		}
	}
	// Blockquote.
	if m := regexpQuote.FindStringSubmatch(line); m != nil {
		return styleMDQuote.Render("│ " + renderMDInline(m[1]))
	}
	// Unordered list.
	if m := regexpList.FindStringSubmatch(line); m != nil {
		return m[1] + styleMDBullet.Render("•") + " " + renderMDInline(m[2])
	}
	// Ordered list.
	if m := regexpOrderedList.FindStringSubmatch(line); m != nil {
		return m[1] + styleMDBullet.Render(m[2]+".") + " " + renderMDInline(m[3])
	}
	return renderMDInline(line)
}

// renderMDInline styles inline spans. Code runs first so its ANSI bytes are
// never re-matched by the bold/italic/link patterns.
func renderMDInline(s string) string {
	s = regexpCode.ReplaceAllStringFunc(s, func(m string) string {
		return styleMDCode.Render(m[1 : len(m)-1])
	})
	s = regexpLink.ReplaceAllStringFunc(s, func(m string) string {
		parts := regexpLink.FindStringSubmatch(m)
		if parts == nil {
			return m
		}
		return styleMDLink.Render(parts[1]) + styleMuted.Render(" ("+parts[2]+")")
	})
	s = regexpBold.ReplaceAllStringFunc(s, func(m string) string {
		return styleMDBold.Render(m[2 : len(m)-2])
	})
	s = regexpItalic.ReplaceAllStringFunc(s, func(m string) string {
		return styleMDItalic.Render(m[1 : len(m)-1])
	})
	return s
}

var (
	regexpHR          = regexp.MustCompile(`^\s*(-{3,}|\*{3,}|_{3,})\s*$`)
	regexpHeading     = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	regexpQuote       = regexp.MustCompile(`^\s*>\s?(.*)$`)
	regexpList        = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	regexpOrderedList = regexp.MustCompile(`^(\s*)(\d{1,3})\.\s+(.*)$`)
	regexpCode        = regexp.MustCompile("`([^`]+)`")
	regexpLink        = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	regexpBold        = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	regexpItalic      = regexp.MustCompile(`\*([^*\n]+)\*`)
)
