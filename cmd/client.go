package cmd

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func daemonSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".einai", "daemon.sock")
}

// newUnixClient returns an HTTP client connected to the einai unix socket.
// No request-level timeout is set — synchronous agent runs can take many minutes.
// Use context cancellation (ctrl-c) to abort long-running requests.
// ResponseHeaderTimeout is set as a safety net so the client does not hang
// indefinitely if the daemon stops responding between accepting the connection
// and writing response headers.
func newUnixClient() *http.Client {
	sock := daemonSocketPath()
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", sock)
			},
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}
}
