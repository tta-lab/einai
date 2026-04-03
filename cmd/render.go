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
	accentColor  = lipgloss.Color("86")  // Teal/cyan
	mutedColor   = lipgloss.Color("245") // Gray
	successColor = lipgloss.Color("82")  // Green
	warningColor = lipgloss.Color("214") // Orange
	errorColor   = lipgloss.Color("196") // Red
)

// Icon styles for command rendering
var (
	successIconStyle = lipgloss.NewStyle().
				Foreground(successColor)
	errorIconStyle = lipgloss.NewStyle().
			Foreground(errorColor)
)

// renderDelta prints the given text to stdout with markdown rendering if TTY.
// Cmd blocks are sent atomically by logos - we discard them here and render in EventCommandResult.
func renderDelta(text string) {
	// Discard cmd blocks - logos sends them atomically, we render with exit status in EventCommandResult
	if strings.HasPrefix(text, logos.CmdBlockOpen) {
		return
	}

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
	// Skip flush for whitespace-only content (can happen when logos sends
	// \n around <cmd> blocks as separate deltas, and we discard the cmd block)
	if strings.TrimSpace(content) == "" {
		rawBuffer.Reset()
		return
	}
	rawBuffer.Reset()

	// Render markdown with glamour
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

// RenderCommand renders a command with its exit status: ✓ $ cmd or ✗ $ cmd
func RenderCommand(command string, exitCode int) {
	icon := successIconStyle.Render("✓")
	if exitCode != 0 {
		icon = errorIconStyle.Render("✗")
	}
	fmt.Printf("%s $ %s\n", icon, command)
}

// Status messages (shown in brackets with subtle styling)
var statusStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("241")).
	Render

// Retry messages
var retryStyle = lipgloss.NewStyle().
	Foreground(warningColor).
	Bold(true)

// Done/checkmark indicator
var doneStyle = lipgloss.NewStyle().
	Foreground(successColor).
	Bold(true)

// Warning indicator
var warningStyle = lipgloss.NewStyle().
	Foreground(warningColor).
	Bold(true)

// renderRetry prints a retry message to stderr.
func renderRetry(reason string, step int) {
	if isTTY {
		stepStr := lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Render(fmt.Sprintf("↻ step %d", step))
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
			cmdStr := lipgloss.NewStyle().
				Foreground(mutedColor).
				Render("running")
			fmt.Fprintf(os.Stderr, "  %s %s...\n", cmdStr, command)
		} else {
			fmt.Fprintf(os.Stderr, "  running %s...\n", command)
		}
	}
}

// renderError prints an error message to stderr with bold red styling.
func renderError(message string) {
	if isTTY {
		errStr := lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Render("✗")
		fmt.Fprintf(os.Stderr, "\n%s %s\n", errStr, message)
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
