package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// NetworkConfig holds network-related sandbox configuration.
type NetworkConfig struct {
	// AllowedDomains is a list of domains that are allowed for network access.
	AllowedDomains []string
}

// SandboxConfig holds sandbox configuration loaded from ~/.config/einai/sandbox.toml.
type SandboxConfig struct {
	// Enabled indicates whether sandboxing is enabled.
	Enabled bool
	// AutoAllowBashIfSandboxed controls whether bash commands are auto-allowed when sandboxed.
	AutoAllowBashIfSandboxed *bool
	// ExcludedCommands lists commands that are excluded from sandboxing.
	ExcludedCommands []string
	// AllowWrite is a list of paths that are allowed to be written.
	AllowWrite []string
	// DenyWrite is a list of paths that are denied from writing.
	DenyWrite []string
	// DenyRead is a list of paths that are denied from reading.
	DenyRead []string
	// AllowRead is a list of paths that are allowed to be read.
	AllowRead []string
	// PermissionsDeny is a list of permissions that are denied.
	PermissionsDeny []string
	// Network holds network configuration.
	Network NetworkConfig
}

// ExpandedAllowWrite returns the AllowWrite paths with ~ expanded.
func (s *SandboxConfig) ExpandedAllowWrite() []string {
	return expandPaths(s.AllowWrite)
}

// ExpandedDenyWrite returns the DenyWrite paths with ~ expanded.
func (s *SandboxConfig) ExpandedDenyWrite() []string {
	return expandPaths(s.DenyWrite)
}

// ExpandedDenyRead returns the DenyRead paths with ~ expanded.
func (s *SandboxConfig) ExpandedDenyRead() []string {
	return expandPaths(s.DenyRead)
}

// ExpandedAllowRead returns the AllowRead paths with ~ expanded.
func (s *SandboxConfig) ExpandedAllowRead() []string {
	return expandPaths(s.AllowRead)
}

// ExpandedPermissionsDeny returns the PermissionsDeny entries with paths expanded.
func (s *SandboxConfig) ExpandedPermissionsDeny() []string {
	return expandPermEntries(s.PermissionsDeny)
}

// expandPaths expands ~ in each path to the user home directory.
func expandPaths(paths []string) []string {
	result := make([]string, len(paths))
	for i, p := range paths {
		result[i] = expandHome(p)
	}
	return result
}

// expandPermEntries expands ~ in permission entries.
// Permission entries have the format "perm:path" or just "perm:".
func expandPermEntries(entries []string) []string {
	result := make([]string, len(entries))
	for i, e := range entries {
		if idx := strings.Index(e, ":"); idx >= 0 && idx < len(e)-1 {
			result[i] = e[:idx+1] + expandHome(e[idx+1:])
		} else {
			result[i] = e
		}
	}
	return result
}

// LoadSandbox loads SandboxConfig from ~/.config/einai/sandbox.toml.
// Returns a default config if the file doesn't exist.
func LoadSandbox() *SandboxConfig {
	cfg, err := LoadSandboxWithError()
	if err != nil {
		log.Printf("warning: failed to load sandbox config: %v", err)
		return &SandboxConfig{}
	}
	return cfg
}

// LoadSandboxWithError loads SandboxConfig from ~/.config/einai/sandbox.toml.
// Returns an error if the file exists but cannot be parsed.
func LoadSandboxWithError() (*SandboxConfig, error) {
	path := filepath.Join(DefaultConfigDir(), "sandbox.toml")
	cfg := &SandboxConfig{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse sandbox.toml: %w", err)
	}
	return cfg, nil
}
