package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tta-lab/einai/internal/event"
)

// streamEndpoint marshals req as JSON, POSTs to the daemon endpoint, and streams
// NDJSON events to stdout/stderr using bubble tea for TTY rendering.
func streamEndpoint(ctx context.Context, endpoint string, req any) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	client := newUnixClient()
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, "http://einai/"+endpoint, bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("daemon unreachable (is 'ei daemon run' running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, bodyErr := io.ReadAll(resp.Body)
		bodyStr := "(could not read response body)"
		if bodyErr == nil {
			bodyStr = strings.TrimSpace(string(body))
		}
		return "", fmt.Errorf("daemon error (%d): %s", resp.StatusCode, bodyStr)
	}

	// Create the output model
	model := newOutputModel()

	// Channel to send events to the tea program
	eventCh := make(chan tea.Msg)

	// Start the tea program in a goroutine
	program := tea.NewProgram(model, tea.WithInput(nil), tea.WithoutRenderer())
	go func() {
		for msg := range eventCh {
			program.Send(msg)
		}
		program.Send(finishMsg{})
	}()

	// Stream events from HTTP response to tea model
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var e event.Event
		if err := json.Unmarshal(line, &e); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] malformed event from daemon: %v\n", err)
			continue
		}

		// Convert event to tea message and send to model
		msg := eventToTeaMsg(e)
		if msg != nil {
			eventCh <- msg
		}
	}

	close(eventCh)

	// Wait for the tea program to finish
	program.Wait()

	return "", nil
}

// eventToTeaMsg converts an event.Event to a tea message.
func eventToTeaMsg(e event.Event) tea.Msg {
	switch e.Type {
	case event.EventDelta:
		return deltaMsg(e.Text)

	case event.EventCommandStart:
		// Command start - handled in command result
		return nil

	case event.EventCommandResult:
		return commandResultMsg{
			Command:  e.Command,
			Output:   e.Output,
			ExitCode: e.ExitCode,
		}

	case event.EventRetry:
		// Retry handled as status
		return statusMsg(fmt.Sprintf("↻ step %d: %s", e.Step, e.Reason))

	case event.EventStatus:
		return statusMsg(e.Message)

	case event.EventError:
		return errorMsg(e.Message)

	case event.EventWarning:
		return warningMsg(e.Message)

	case event.EventRateLimit:
		if e.RateLimitRetryAfter > 0 {
			return statusMsg(fmt.Sprintf("rate limited, retrying in %d seconds...", e.RateLimitRetryAfter))
		}
		return warningMsg("rate limited - please wait before retrying")

	case event.EventDone:
		return finishMsg{}

	default:
		return nil
	}
}

// handleEvent processes a single event and updates the response if needed.
// This is kept for backward compatibility with tests.
func handleEvent(e event.Event, response *string) error {
	switch e.Type {
	case event.EventDelta:
		renderDelta(e.Text)

	case event.EventCommandStart:
		renderCommandStart(e.Command)

	case event.EventCommandResult:
		RenderCommand(e.Command, e.Output, e.ExitCode)

	case event.EventRetry:
		renderRetry(e.Reason, e.Step)

	case event.EventStatus:
		renderStatus(e.Message)

	case event.EventError:
		renderError(e.Message)
		return errors.New(e.Message)

	case event.EventWarning:
		renderWarning(e.Message)

	case event.EventRateLimit:
		if e.RateLimitRetryAfter > 0 {
			renderStatus(fmt.Sprintf("rate limited, retrying in %d seconds...", e.RateLimitRetryAfter))
		} else {
			renderWarning("rate limited - please wait before retrying")
		}

	case event.EventDone:
		FlushDelta()
		*response = e.Response
		if *response != "" {
			fmt.Println()
		}
		renderDone()

	default:
		fmt.Fprintf(os.Stderr, "[warn] unhandled event type: %s\n", e.Type)
	}
	return nil
}
