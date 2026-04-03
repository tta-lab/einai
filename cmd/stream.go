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

	"github.com/tta-lab/einai/internal/event"
)

// streamEndpoint marshals req as JSON, POSTs to the daemon endpoint, and streams
// NDJSON events to stdout/stderr.
// Returns the final response string and any error encountered.
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

	var response string
	var sessionErr error
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
		sessionErr = handleEvent(e, &response)
		if sessionErr != nil {
			break
		}
	}

	// Flush any remaining buffered markdown
	FlushDelta()

	// Log scanner errors if no session error was already captured
	if sessionErr == nil {
		if scanErr := scanner.Err(); scanErr != nil {
			fmt.Fprintf(os.Stderr, "[warn] stream read error: %v\n", scanErr)
		}
	}

	// If no done event was received and no error occurred, show done indicator
	if sessionErr == nil && response == "" {
		renderDone()
	}

	return response, sessionErr
}

// handleEvent processes a single event and updates the response if needed.
// Returns an error if the event indicates a session failure.
func handleEvent(e event.Event, response *string) error {
	switch e.Type {
	case event.EventDelta:
		// Main content stream - pass through to stdout
		renderDelta(e.Text)

	case event.EventCommandStart:
		// Command is starting - show styled indicator
		renderCommandStart(e.Command)

	case event.EventCommandResult:
		// Command completed - render with exit status
		RenderCommand(e.Command, e.ExitCode)

	case event.EventRetry:
		// Model is retrying - show retry indicator
		renderRetry(e.Reason, e.Step)

	case event.EventStatus:
		// Status updates - show as subtle inline messages
		renderStatus(e.Message)

	case event.EventError:
		// Errors - show with red styling and return error
		renderError(e.Message)
		return errors.New(e.Message)

	case event.EventWarning:
		// Warnings - show with warning styling
		renderWarning(e.Message)

	case event.EventRateLimit:
		// Rate limit - show with info about retry
		if e.RateLimitRetryAfter > 0 {
			renderStatus(fmt.Sprintf("rate limited, retrying in %d seconds...", e.RateLimitRetryAfter))
		} else {
			renderWarning("rate limited - please wait before retrying")
		}

	case event.EventDone:
		// Flush any remaining buffered markdown before showing response
		FlushDelta()
		// Session complete - show done indicator
		*response = e.Response
		if *response != "" {
			fmt.Println()
		}
		renderDone()

	default:
		// Log unhandled event types to prevent silent failures on protocol changes
		fmt.Fprintf(os.Stderr, "[warn] unhandled event type: %s\n", e.Type)
	}
	return nil
}
