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

// ndjsonStream wraps an NDJSON response body for reading events.
type ndjsonStream struct {
	scanner  *bufio.Scanner
	response io.ReadCloser
}

// newNDJSONStream creates a new NDJSON stream from an HTTP response.
func newNDJSONStream(body io.ReadCloser) *ndjsonStream {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	return &ndjsonStream{
		scanner:  scanner,
		response: body,
	}
}

// readEventCmd returns a tea.Cmd that reads one event from the stream.
func (s *ndjsonStream) readEventCmd() tea.Cmd {
	return func() tea.Msg {
		if !s.scanner.Scan() {
			// Stream exhausted
			if err := s.scanner.Err(); err != nil {
				return errorMsg(fmt.Sprintf("stream error: %v", err))
			}
			return finishMsg{}
		}

		line := s.scanner.Bytes()
		if len(line) == 0 {
			return s.readEventCmd() // Try again
		}

		var e event.Event
		if err := json.Unmarshal(line, &e); err != nil {
			return warningMsg(fmt.Sprintf("malformed event: %v", err))
		}

		return eventToTeaMsg(e)
	}
}

// streamEndpoint marshals req as JSON, POSTs to the daemon endpoint, and streams
// NDJSON events using bubble tea for TTY rendering.
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

	// Wrap the response body in our NDJSON stream
	stream := newNDJSONStream(resp.Body)
	model.SetStream(stream)

	// Build tea program options
	var opts []tea.ProgramOption
	opts = append(opts, tea.WithInput(nil))
	if !isOutputTTY() {
		opts = append(opts, tea.WithoutRenderer())
	}

	// Run the tea program
	program := tea.NewProgram(model, opts...)
	if _, err := program.Run(); err != nil {
		return "", fmt.Errorf("tea program error: %w", err)
	}

	return "", nil
}

// eventToTeaMsg converts an event.Event to a tea message.
func eventToTeaMsg(e event.Event) tea.Msg {
	switch e.Type {
	case event.EventDelta:
		return deltaMsg(e.Text)

	case event.EventCommandStart:
		return commandStartMsg{Command: e.Command}

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
