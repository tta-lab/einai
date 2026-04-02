package cmd

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

const maxOutputLines = 10

// retryStyle returns the style for displaying retry messages.
var retryStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("214")).
	Bold(true)

// outputStyle returns the style for displaying truncated command output.
var outputStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("245")).
	PaddingLeft(2)

// renderCommandResult prints command execution results to stderr.
// On failure (exitCode != 0), it prints the command line and truncated output.
// On success, it only prints the command line.
func renderCommandResult(command, output string, exitCode int) {
	cmdLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Bold(true).
		Render("  $ " + command)
	fmt.Fprintln(os.Stderr, cmdLine)

	if exitCode != 0 {
		truncated := truncateOutput(output)
		if truncated != "" {
			fmt.Fprintf(os.Stderr, "%s\n", outputStyle.Render(truncated))
		}
		exitStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		exitLine := exitStyle.Render(fmt.Sprintf("  exit %d", exitCode))
		fmt.Fprintln(os.Stderr, exitLine)
	}
}

// renderRetry prints a retry message to stderr.
func renderRetry(reason string, step int) {
	fmt.Fprintf(os.Stderr, "%s\n", retryStyle.Render(fmt.Sprintf("↺ retry (step %d: %s)", step, reason)))
}

// renderDelta prints the given text to stdout (pass-through, no styling).
func renderDelta(text string) {
	fmt.Print(text)
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
