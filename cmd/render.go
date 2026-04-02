package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
)

const maxOutputLines = 10

// TTY detection - lazy initialization like mods
var isTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})()

// Markdown buffer for streaming - we buffer until we see a newline before rendering
var deltaBuffer strings.Builder

// Reusable glamour renderer - lazy initialization
var markdownRenderer = sync.OnceValue(func() *glamour.TermRenderer {
	r, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return nil
	}
	return r
})

// Regex patterns for special markers
var (
	cmdBlockRegex = regexp.MustCompile(`(?m)<cmd>|</cmd>`)
	sectionRegex  = regexp.MustCompile(`§\s*`)
)

// renderDelta prints the given text to stdout with markdown rendering if TTY.
// It buffers input and renders at newline boundaries for streaming-friendly output.
func renderDelta(text string) {
	if !isTTY {
		// Non-TTY: pass through raw text for agents
		fmt.Print(text)
		return
	}

	// TTY: buffer and render markdown at newlines
	deltaBuffer.WriteString(text)

	// Flush complete lines for markdown rendering
	for {
		s := deltaBuffer.String()
		idx := strings.Index(s, "\n")
		if idx < 0 {
			break
		}
		line := s[:idx]
		deltaBuffer.Reset()
		deltaBuffer.WriteString(s[idx+1:])
		renderMarkdownLine(line)
	}
}

// FlushDelta renders any remaining buffered content as markdown.
func FlushDelta() {
	remaining := deltaBuffer.String()
	if remaining == "" {
		return
	}
	deltaBuffer.Reset()
	if isTTY {
		renderMarkdownLine(remaining)
	} else {
		fmt.Print(remaining)
	}
}

// renderMarkdownLine renders a single line of text as markdown with glamour.
func renderMarkdownLine(text string) {
	if text == "" {
		fmt.Println()
		return
	}

	// Clean model-specific markers
	text = cleanModelMarkers(text)
	if text == "" {
		return
	}

	// Use glamour for rendering
	if r := markdownRenderer(); r != nil {
		out, err := r.Render(text)
		if err == nil {
			// Glamour adds trailing newline, trim to avoid double
			fmt.Print(strings.TrimSuffix(out, "\n"))
			return
		}
	}

	// Fallback: simple styling
	renderSimpleMarkdown(text)
}

// cleanModelMarkers removes or transforms model-specific markers for cleaner display.
func cleanModelMarkers(text string) string {
	// Remove <cmd> and </cmd> tags
	text = cmdBlockRegex.ReplaceAllString(text, "")

	// Remove § command prefix
	text = sectionRegex.ReplaceAllString(text, "")

	return strings.TrimSpace(text)
}

// renderSimpleMarkdown applies simple inline markdown styles (fallback).
func renderSimpleMarkdown(text string) {
	var result string

	// Headers
	if strings.HasPrefix(text, "# ") {
		result = headerStyle.Render(text[2:])
	} else if strings.HasPrefix(text, "## ") {
		result = subheaderStyle.Render(text[3:])
	} else if strings.HasPrefix(text, "```") {
		result = codeBlockStyle.Render(text)
	} else if strings.HasPrefix(text, "- ") || strings.HasPrefix(text, "* ") {
		result = bulletStyle.Render(text)
	} else if strings.HasPrefix(text, "> ") {
		result = quoteStyle.Render(text[2:])
	} else {
		result = styleInlineMarkdown(text)
	}

	fmt.Println(result)
}

// styleInlineMarkdown applies inline markdown styles (bold, italic, code).
func styleInlineMarkdown(text string) string {
	// Bold: **text** or __text__
	var result = text
	boldStyle := lipgloss.NewStyle().Bold(true)
	for {
		start := strings.Index(result, "**")
		if start < 0 {
			start = strings.Index(result, "__")
		}
		if start < 0 {
			break
		}
		end := strings.Index(result[start+2:], "**")
		if end < 0 {
			end = strings.Index(result[start+2:], "__")
		}
		if end < 0 {
			break
		}
		inner := result[start+2 : start+2+end]
		result = result[:start] + boldStyle.Render(inner) + result[start+2+end+2:]
	}

	// Inline code: `code`
	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("201")).
		Background(lipgloss.Color("236")).
		Render
	for {
		start := strings.Index(result, "`")
		if start < 0 {
			break
		}
		end := strings.Index(result[start+1:], "`")
		if end < 0 {
			break
		}
		inner := result[start+1 : start+1+end]
		result = result[:start] + codeStyle(inner) + result[start+1+end+1:]
	}

	return result
}

// Color palette
var (
	// Primary colors
	accentColor  = lipgloss.Color("86")  // Teal/cyan for highlights
	mutedColor   = lipgloss.Color("245") // Gray for secondary text
	successColor = lipgloss.Color("82")  // Green for success
	warningColor = lipgloss.Color("214") // Orange for warnings
	errorColor   = lipgloss.Color("196") // Red for errors
)

// Styles
var (
	// Status messages (shown in brackets with subtle styling)
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render

	// Retry messages
	retryStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	// Command output (truncated, dimmed)
	outputStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(2)

	// Error messages
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Exit code indicator
	exitStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Command line (shown when command fails)
	commandStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Bold(true)

	// Step indicator
	stepStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	// Done/checkmark indicator
	doneStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	// Markdown styles
	headerStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	subheaderStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Bold(true)

	codeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	bulletStyle = lipgloss.NewStyle().
			Foreground(accentColor)

	quoteStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)

	// Command running indicator
	commandStartStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Warning indicator
	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)
)

// renderCommandResult prints command execution results to stderr.
// On failure (exitCode != 0), it prints the command line, truncated output, and exit code.
// On success (exitCode == 0), nothing is printed to avoid duplicating the command that
// the LLM already shows in its response (logos includes the command in the tool result).
func renderCommandResult(command, output string, exitCode int) {
	if exitCode == 0 {
		return
	}

	cmdLine := commandStyle.Render("$ " + command)
	fmt.Fprintf(os.Stderr, "  %s\n", cmdLine)

	truncated := truncateOutput(output)
	if truncated != "" {
		fmt.Fprintf(os.Stderr, "%s\n", outputStyle.Render(truncated))
	}
	exitLine := exitStyle.Render(fmt.Sprintf("  ✗ exit %d", exitCode))
	fmt.Fprintln(os.Stderr, exitLine)
}

// renderRetry prints a retry message to stderr.
func renderRetry(reason string, step int) {
	stepStr := stepStyle.Render(fmt.Sprintf("↻ step %d", step))
	reasonStr := retryStyle.Render(reason)
	fmt.Fprintf(os.Stderr, "%s · %s\n", stepStr, reasonStr)
}

// renderStatus prints a status message to stderr with subtle styling.
func renderStatus(message string) {
	// Format as a subtle inline status
	fmt.Fprintf(os.Stderr, "%s %s\n", statusStyle("···"), message)
}

// renderWarning prints a warning message to stderr with warning styling.
func renderWarning(message string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", warningStyle.Render("⚠"), message)
}

// renderCommandStart prints a command start indicator to stderr.
func renderCommandStart(command string) {
	if command != "" {
		cmdStr := commandStartStyle.Render("running")
		fmt.Fprintf(os.Stderr, "  %s %s...\n", cmdStr, command)
	}
}

// renderError prints an error message to stderr with bold red styling.
func renderError(message string) {
	fmt.Fprintf(os.Stderr, "\n%s %s\n", errorStyle.Render("✗"), message)
}

// renderDone prints a done/finish indicator.
func renderDone() {
	fmt.Fprintln(os.Stderr, "\n"+doneStyle.Render("✓ done"))
}

// truncateOutput limits output to 10 lines plus a summary of any remaining lines.
func truncateOutput(output string) string {
	lines := strings.Split(output, "\n")

	// Count non-empty trailing lines for the "... (N more lines)" message
	extraLines := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			break
		}
		extraLines++
	}

	// Count meaningful lines (excluding trailing empty lines)
	meaningfulLines := len(lines) - extraLines
	if meaningfulLines > maxOutputLines {
		truncated := strings.Join(lines[:maxOutputLines], "\n")
		remaining := meaningfulLines - maxOutputLines
		if remaining == 1 {
			truncated += "\n... (1 more line)"
		} else {
			truncated += fmt.Sprintf("\n... (%d more lines)", remaining)
		}
		return truncated
	}

	return output
}
