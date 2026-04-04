package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
)

const tabWidth = 4

// Style definitions for TTY output
var (
	exitStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	pendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render
)

// outputModel is the Bubble Tea model for rendering agent output.
// Uses tea.Cmd pattern to read NDJSON events without deadlock.
type outputModel struct {
	output     strings.Builder // raw accumulated output for glamour
	glamOutput string          // glamour-rendered output (markdown only)
	glamHeight int
	viewport   viewport.Model
	glam       *glamour.TermRenderer
	width      int
	height     int
	finished   bool

	// Stream for reading events
	stream *ndjsonStream

	// Pending command shown at bottom of output
	pendingCmd    string
	hasPendingCmd bool
}

// newOutputModel creates a new output model.
func newOutputModel() *outputModel {
	glam, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(100),
	)

	m := &outputModel{
		glam: glam,
	}

	vp := viewport.New()
	vp.GotoBottom()
	m.viewport = vp

	return m
}

// SetStream sets the NDJSON stream for reading events.
func (m *outputModel) SetStream(stream *ndjsonStream) {
	m.stream = stream
}

// Init implements tea.Model.
func (m *outputModel) Init() tea.Cmd {
	if m.stream != nil {
		return m.stream.readEventCmd()
	}
	return nil
}

// Update implements tea.Model.
func (m *outputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height)
		return m, nil

	case deltaMsg:
		m.appendDelta(string(msg))
		if m.stream != nil {
			return m, m.stream.readEventCmd()
		}
		return m, nil

	case commandStartMsg:
		m.appendCommandStart(msg.Command)
		if m.stream != nil {
			return m, m.stream.readEventCmd()
		}
		return m, nil

	case commandResultMsg:
		m.appendCommandResult(msg.Command, msg.Output, msg.ExitCode)
		if m.stream != nil {
			return m, m.stream.readEventCmd()
		}
		return m, nil

	case statusMsg:
		m.appendStatus(string(msg))
		if m.stream != nil {
			return m, m.stream.readEventCmd()
		}
		return m, nil

	case warningMsg:
		m.appendWarning(string(msg))
		if m.stream != nil {
			return m, m.stream.readEventCmd()
		}
		return m, nil

	case errorMsg:
		m.appendError(string(msg))
		return m, tea.Quit

	case finishMsg:
		m.finished = true
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	// Update viewport
	m.viewport, _ = m.viewport.Update(msg)
	return m, nil
}

// View implements tea.Model.
func (m *outputModel) View() tea.View {
	// For non-TTY, content is printed directly in append methods
	if !isOutputTTY() {
		return tea.NewView("")
	}

	// Build full output: glamour output + pending command
	fullOutput := m.glamOutput
	if m.hasPendingCmd {
		fullOutput += fmt.Sprintf("%s $ %s", pendingStyle("···"), m.pendingCmd)
	}

	// On finish, print to stdout before returning empty (program will quit)
	if m.finished {
		fmt.Print(fullOutput)
		return tea.NewView("")
	}

	// TTY: use viewport if content exceeds screen
	fullHeight := lipgloss.Height(fullOutput)
	if fullHeight > m.viewport.Height() {
		m.viewport.SetContent(fullOutput)
		return tea.NewView(m.viewport.View())
	}

	// Content fits on screen
	return tea.NewView(fullOutput)
}

// appendDelta appends streaming delta content.
func (m *outputModel) appendDelta(text string) {
	if !isOutputTTY() {
		// Non-TTY: just pass through directly
		fmt.Print(text)
		return
	}

	// TTY: render with glamour
	m.output.WriteString(text)
	m.renderGlamour()
}

// appendCommandStart shows a pending command indicator.
func (m *outputModel) appendCommandStart(command string) {
	// Track the pending command
	m.pendingCmd = command
	m.hasPendingCmd = true

	if !isOutputTTY() {
		return
	}

	// For TTY, pending command is rendered in View()
	// Just update viewport height for proper scrolling
	m.glamHeight = lipgloss.Height(m.glamOutput)
}

// appendCommandResult appends a command result.
func (m *outputModel) appendCommandResult(command, output string, exitCode int) {
	// Clear pending state
	m.hasPendingCmd = false
	m.pendingCmd = ""

	if !isOutputTTY() {
		// Non-TTY: print raw
		icon := "✓"
		if exitCode != 0 {
			icon = "✗"
		}
		fmt.Printf("%s $ %s\n", icon, command)
		if exitCode != 0 && output != "" {
			fmt.Printf("%s\n", truncateOutput(output))
			fmt.Printf("  exit %d\n", exitCode)
		}
		return
	}

	// TTY: styled output
	icon := successIconStyle.Render("✓")
	if exitCode != 0 {
		icon = errorIconStyle.Render("✗")
	}
	m.glamOutput += fmt.Sprintf("%s $ %s\n", icon, command)

	if exitCode != 0 && output != "" {
		truncated := truncateOutput(output)
		if truncated != "" {
			m.glamOutput += fmt.Sprintf("%s\n", outputStyle.Render(truncated))
		}
		m.glamOutput += fmt.Sprintf("%s\n", exitStyle.Render(fmt.Sprintf("  exit %d", exitCode)))
	}

	m.glamHeight = lipgloss.Height(m.glamOutput)
	m.updateViewport()
}

// appendStatus appends a status message.
func (m *outputModel) appendStatus(msg string) {
	if !isOutputTTY() {
		fmt.Fprintf(os.Stderr, "··· %s\n", msg)
		return
	}
	m.glamOutput += statusStyle("··· "+msg) + "\n"
	m.glamHeight = lipgloss.Height(m.glamOutput)
}

// appendWarning appends a warning message.
func (m *outputModel) appendWarning(msg string) {
	if !isOutputTTY() {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", msg)
		return
	}
	m.glamOutput += warningStyle.Render("⚠") + " " + msg + "\n"
	m.glamHeight = lipgloss.Height(m.glamOutput)
}

// appendError appends an error message.
func (m *outputModel) appendError(msg string) {
	if !isOutputTTY() {
		fmt.Fprintf(os.Stderr, "\n✗ %s\n", msg)
		return
	}
	m.glamOutput += errorIconStyle.Render("✗") + " " + msg + "\n"
	m.glamHeight = lipgloss.Height(m.glamOutput)
}

// updateViewport updates the viewport content and scrolls to bottom if needed.
func (m *outputModel) updateViewport() {
	wasAtBottom := m.viewport.AtBottom()
	oldHeight := m.glamHeight

	m.viewport.SetContent(m.glamOutput)

	if oldHeight < m.glamHeight && wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// renderGlamour re-renders the output with glamour.
func (m *outputModel) renderGlamour() {
	if m.glam == nil {
		return
	}

	glamOut, err := m.glam.Render(m.output.String())
	if err != nil {
		return
	}

	// Trim trailing whitespace like mods
	glamOut = strings.TrimRightFunc(glamOut, unicode.IsSpace)
	glamOut = strings.ReplaceAll(glamOut, "\t", strings.Repeat(" ", tabWidth))

	m.glamOutput = glamOut + "\n"
	m.glamHeight = lipgloss.Height(m.glamOutput)
	m.updateViewport()
}

// Message types for tea
type deltaMsg string
type commandResultMsg struct {
	Command  string
	Output   string
	ExitCode int
}
type commandStartMsg struct {
	Command string
}
type statusMsg string
type warningMsg string
type errorMsg string
type finishMsg struct{}

// isOutputTTY checks if stdout is a terminal.
var isOutputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})
