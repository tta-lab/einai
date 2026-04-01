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

func newUnixClient() *http.Client {
	sock := daemonSocketPath()
	return &http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", sock)
			},
		},
	}
}
