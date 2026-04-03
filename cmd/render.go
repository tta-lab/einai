package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/tta-lab/logos"
)

const maxOutputLines = 10

// TTY detection - lazy initialization like mods
var isTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})()

// Markdown buffer for streaming - accumulate until we have complete blocks
var rawBuffer strings.Builder

// Reusable glamour renderer - lazy initialization
var markdownRenderer = sync.OnceValue(func() *glamour.TermRenderer {
	r, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		log.Printf("[render] failed to initialize markdown renderer: %v", err)
		return nil
	}
	return r
})

// Color palette
var (
	accentColor   = lipgloss.Color("86")  // Teal/cyan
	mutedColor    = lipgloss.Color("245") // Gray
	successColor  = lipgloss.Color("82")  // Green
	warningColor  = lipgloss.Color("214") // Orange
	errorColor    = lipgloss.Color("196") // Red
	blueColor     = lipgloss.Color("75")  // Blue
	bgBaseLighter = lipgloss.Color("236") // Darker background for code
)

// Tool styles - mirrors crush's styling
var (
	toolIconStyle = lipgloss.NewStyle().
			Foreground(successColor)

	toolNameStyle = lipgloss.NewStyle().
			Foreground(blueColor)

	toolBodyStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	toolContentStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Background(bgBaseLighter)
)

// renderDelta prints the given text to stdout with markdown rendering if TTY.
// For streaming, we buffer content and render in chunks at meaningful boundaries.
func renderDelta(text string) {
	// Non-TTY: pass through raw text for agents
	if !isTTY {
		fmt.Print(text)
		return
	}

	// TTY: buffer content
	rawBuffer.WriteString(text)

	// Check if we should flush - look for block boundaries
	// Flush on: double newline (paragraph break), or end of code block
	content := rawBuffer.String()

	// Flush on paragraph boundary (double newline)
	shouldFlush := strings.Contains(content, "\n\n")

	// Flush on code block end
	if strings.HasSuffix(content, "```\n") || strings.HasSuffix(content, "```\r\n") {
		shouldFlush = true
	}

	// Flush if we have incomplete content (ends with newline but no double newline)
	// This keeps output streaming
	if !shouldFlush && len(content) > 0 && (strings.HasSuffix(content, "\n") || strings.HasSuffix(content, "\r\n")) {
		// Count newlines - if we have a complete line without content following, flush it
		lines := strings.Split(content, "\n")
		if len(lines) > 1 && lines[len(lines)-1] == "" {
			shouldFlush = true
		}
	}

	if shouldFlush {
		flushBuffer()
	}
}

// flushBuffer renders the buffered content as a complete markdown document.
func flushBuffer() {
	content := rawBuffer.String()
	if content == "" {
		return
	}
	rawBuffer.Reset()

	// Clean model-specific markers
	content = cleanModelMarkers(content)

	// Render with glamour as complete markdown
	if r := markdownRenderer(); r != nil {
		out, err := r.Render(content)
		if err == nil {
			fmt.Print(out)
			return
		}
		log.Printf("[render] markdown render error: %v", err)
	} else {
		log.Printf("[render] markdown renderer not available")
	}

	// Fallback: pass through
	fmt.Print(content)
}

// FlushDelta renders any remaining buffered content as markdown.
func FlushDelta() {
	flushBuffer()
}

// cleanModelMarkers transforms model-specific markers for cleaner display.
// Mirrors crush's tool rendering: header + body with proper lipgloss styling.
func cleanModelMarkers(text string) string {
	// Check if there are any cmd blocks
	if !strings.Contains(text, logos.CmdBlockOpen) {
		return strings.TrimSpace(text)
	}

	// Extract all content from <cmd>...</cmd> blocks
	var parts []string
	remaining := text
	for {
		openIdx := strings.Index(remaining, logos.CmdBlockOpen)
		if openIdx == -1 {
			break
		}
		remaining = remaining[openIdx+len(logos.CmdBlockOpen):]
		closeIdx := strings.Index(remaining, logos.CmdBlockClose)
		if closeIdx == -1 {
			// Unclosed block - take rest as content
			parts = append(parts, remaining)
			break
		}
		content := remaining[:closeIdx]
		remaining = remaining[closeIdx+len(logos.CmdBlockClose):]
		parts = append(parts, content)
	}

	if len(parts) == 0 {
		return strings.TrimSpace(text)
	}

	// Build header: ● Bash $
	header := toolIconStyle.Render("●") + " " + toolNameStyle.Render("Bash") + " $"

	// Build body with proper styling
	content := strings.TrimSpace(strings.Join(parts, "\n"))
	lines := strings.Split(content, "\n")

	var bodyLines []string
	for _, ln := range lines {
		bodyLines = append(bodyLines, toolContentStyle.Render(" "+ln))
	}
	body := toolBodyStyle.Render(strings.Join(bodyLines, "\n"))

	// Join header and body like crush does
	return header + "\n" + body
}

// Status messages (shown in brackets with subtle styling)
var statusStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("241")).
	Render

// Retry messages
var retryStyle = lipgloss.NewStyle().
	Foreground(warningColor).
	Bold(true)

// Command output (truncated, dimmed)
var outputStyle = lipgloss.NewStyle().
	Foreground(mutedColor).
	PaddingLeft(2)

// Error messages
var errorStyle = lipgloss.NewStyle().
	Foreground(errorColor).
	Bold(true)

// Exit code indicator
var exitStyle = lipgloss.NewStyle().
	Foreground(errorColor).
	Bold(true)

// Command line (shown when command fails)
var commandStyle = lipgloss.NewStyle().
	Foreground(mutedColor).
	Bold(true)

// Step indicator
var stepStyle = lipgloss.NewStyle().
	Foreground(accentColor).
	Bold(true)

// Done/checkmark indicator
var doneStyle = lipgloss.NewStyle().
	Foreground(successColor).
	Bold(true)

// Command running indicator
var commandStartStyle = lipgloss.NewStyle().
	Foreground(mutedColor)

// Warning indicator
var warningStyle = lipgloss.NewStyle().
	Foreground(warningColor).
	Bold(true)

// renderCommandResult prints command execution results to stderr.
// On failure (exitCode != 0), it prints the command line, truncated output, and exit code.
// On success (exitCode == 0), nothing is printed to avoid duplicating the command that
// the LLM already shows in its response (logos includes the command in the tool result).
func renderCommandResult(command, output string, exitCode int) {
	if exitCode == 0 {
		return
	}

	if isTTY {
		cmdLine := commandStyle.Render("$ " + command)
		fmt.Fprintf(os.Stderr, "  %s\n", cmdLine)
		truncated := truncateOutput(output)
		if truncated != "" {
			fmt.Fprintf(os.Stderr, "%s\n", outputStyle.Render(truncated))
		}
		exitLine := exitStyle.Render(fmt.Sprintf("  ✗ exit %d", exitCode))
		fmt.Fprintln(os.Stderr, exitLine)
	} else {
		fmt.Fprintf(os.Stderr, "  $ %s\n", command)
		truncated := truncateOutput(output)
		if truncated != "" {
			fmt.Fprintf(os.Stderr, "%s\n", truncated)
		}
		fmt.Fprintf(os.Stderr, "  ✗ exit %d\n", exitCode)
	}
}

// renderRetry prints a retry message to stderr.
func renderRetry(reason string, step int) {
	if isTTY {
		stepStr := stepStyle.Render(fmt.Sprintf("↻ step %d", step))
		reasonStr := retryStyle.Render(reason)
		fmt.Fprintf(os.Stderr, "%s · %s\n", stepStr, reasonStr)
	} else {
		fmt.Fprintf(os.Stderr, "↻ step %d · %s\n", step, reason)
	}
}

// renderStatus prints a status message to stderr with subtle styling.
func renderStatus(message string) {
	if isTTY {
		fmt.Fprintf(os.Stderr, "%s %s\n", statusStyle("···"), message)
	} else {
		fmt.Fprintf(os.Stderr, "··· %s\n", message)
	}
}

// renderWarning prints a warning message to stderr with warning styling.
func renderWarning(message string) {
	if isTTY {
		fmt.Fprintf(os.Stderr, "%s %s\n", warningStyle.Render("⚠"), message)
	} else {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", message)
	}
}

// renderCommandStart prints a command start indicator to stderr.
func renderCommandStart(command string) {
	if command != "" {
		if isTTY {
			cmdStr := commandStartStyle.Render("running")
			fmt.Fprintf(os.Stderr, "  %s %s...\n", cmdStr, command)
		} else {
			fmt.Fprintf(os.Stderr, "  running %s...\n", command)
		}
	}
}

// renderError prints an error message to stderr with bold red styling.
func renderError(message string) {
	if isTTY {
		fmt.Fprintf(os.Stderr, "\n%s %s\n", errorStyle.Render("✗"), message)
	} else {
		fmt.Fprintf(os.Stderr, "\n✗ %s\n", message)
	}
}

// renderDone prints a done/finish indicator.
func renderDone() {
	if isTTY {
		fmt.Fprintln(os.Stderr, doneStyle.Render("✓ done"))
	}
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
