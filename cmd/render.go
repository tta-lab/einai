package cmd

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// commandStyle returns the style for displaying the command line.
var commandStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("241")).
	Bold(true)

// exitCodeStyle returns the style for displaying the exit code.
var exitCodeStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("196")).
	Bold(true)

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
	fmt.Fprintf(os.Stderr, "  $ %s\n", command)

	if exitCode != 0 {
		truncated := truncateOutput(output)
		if truncated != "" {
			fmt.Fprintf(os.Stderr, "%s\n", outputStyle.Render(truncated))
		}
		fmt.Fprintf(os.Stderr, "  exit %d\n", exitCode)
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

// truncateOutput limits the output to 10 lines.
// If there are more lines, it appends a "... (N more lines)" line.
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
	if meaningfulLines > 10 {
		truncated := strings.Join(lines[:10], "\n")
		remaining := meaningfulLines - 10
		if remaining == 1 {
			truncated += "\n... (1 more line)"
		} else {
			truncated += fmt.Sprintf("\n... (%d more lines)", remaining)
		}
		return truncated
	}

	return output
}
