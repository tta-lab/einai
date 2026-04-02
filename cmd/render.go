package cmd

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

const maxOutputLines = 10

// Color palette
var (
	// Primary colors
	accentColor  = lipgloss.Color("86")  // Teal/cyan for highlights
	mutedColor   = lipgloss.Color("245") // Gray for secondary text
	successColor = lipgloss.Color("82")  // Green for success
	warningColor = lipgloss.Color("214") // Orange for warnings
	errorColor   = lipgloss.Color("196") // Red for errors
	commandColor = lipgloss.Color("75")  // Blue for command output
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

// renderDelta prints the given text to stdout (pass-through, no styling).
// This is the main content stream from the model.
func renderDelta(text string) {
	fmt.Print(text)
}

// renderStatus prints a status message to stderr with subtle styling.
func renderStatus(message string) {
	// Format as a subtle inline status
	fmt.Fprintf(os.Stderr, "%s %s\n", statusStyle("···"), message)
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
