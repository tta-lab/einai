package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/tta-lab/einai/internal/event"
)

// streamEndpoint marshals req as JSON, POSTs to the daemon endpoint, and streams
// NDJSON events to stdout/stderr. errPrefix is used in the error event message.
func streamEndpoint(ctx context.Context, endpoint string, req any, errPrefix string) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	client := newUnixClient()
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, "http://einai/"+endpoint, bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("daemon unreachable (is 'ei daemon run' running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e event.Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		switch e.Type {
		case event.EventDelta:
			fmt.Print(e.Text)
		case event.EventStatus:
			fmt.Fprintf(os.Stderr, "\n[%s]\n", e.Message)
		case event.EventError:
			fmt.Fprintf(os.Stderr, "\nError: %s\n", e.Message)
			return fmt.Errorf("%s: %s", errPrefix, e.Message)
		case event.EventDone:
			fmt.Println()
		}
	}
	return scanner.Err()
}
