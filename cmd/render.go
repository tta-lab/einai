package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"unicode"

	"charm.land/glamour/v2"
	"github.com/mattn/go-isatty"
)

// isTTY returns true if stdout is a terminal.
var isTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})()

// markdownRenderer is a reusable glamour renderer.
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

// renderResult renders the final text result to stdout using glamour.
// On non-TTY, passes through raw text. On TTY, renders as markdown.
func renderResult(text string) error {
	if text == "" {
		return nil
	}

	if !isTTY {
		fmt.Print(text)
		if !strings.HasSuffix(text, "\n") {
			fmt.Println()
		}
		return nil
	}

	r := markdownRenderer()
	if r == nil {
		fmt.Print(text)
		return nil
	}

	out, err := r.Render(text)
	if err != nil {
		// Fallback to raw output
		fmt.Print(text)
		return nil
	}

	// Trim trailing whitespace like mods
	out = strings.TrimRightFunc(out, unicode.IsSpace)
	fmt.Println(out)
	return nil
}

// truncateOutput limits output to 10 lines plus a summary of remaining lines.
func truncateOutput(output string) string {
	const maxOutputLines = 10
	lines := strings.Split(output, "\n")

	extraLines := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			break
		}
		extraLines++
	}

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
