package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	defaultModel     = "claude-sonnet-4-6"
	defaultMaxSteps  = 100
	defaultMaxTokens = 131072
)

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	RequestsPerMinute  int `toml:"requests_per_minute"`
	ConcurrentSessions int `toml:"concurrent_sessions"`
}

// EinaiConfig holds einai daemon configuration loaded from ~/.config/einai/config.toml.
type EinaiConfig struct {
	// Local path for cloned OSS reference repos (default: ~/.einai/references/)
	ReferencesPath string `toml:"references_path"`
	// Model for the agent loop (default: claude-sonnet-4-6)
	Model string `toml:"model"`
	// Maximum agent steps (default: 100)
	MaxSteps int `toml:"max_steps"`
	// Maximum output tokens per step (default: 131072)
	MaxTokens int `toml:"max_tokens"`
	// Paths to search for agent .md files
	AgentsPaths []string `toml:"agents_paths"`
	// Rate limiting configuration
	RateLimit RateLimitConfig `toml:"rate_limit"`
}

// AgentModel returns the configured model or default.
func (c *EinaiConfig) AgentModel() string {
	if c.Model != "" {
		return c.Model
	}
	return defaultModel
}

// AgentMaxSteps returns the configured max steps or default.
func (c *EinaiConfig) AgentMaxSteps() int {
	if c.MaxSteps > 0 {
		return c.MaxSteps
	}
	return defaultMaxSteps
}

// AgentMaxTokens returns the configured max tokens or default.
func (c *EinaiConfig) AgentMaxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	return defaultMaxTokens
}

// AgentReferencesPath returns the configured references path or default.
func (c *EinaiConfig) AgentReferencesPath() string {
	if c.ReferencesPath != "" {
		return expandHome(c.ReferencesPath)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".einai", "references")
}

// RateLimitRequestsPerMinute returns the configured requests per minute limit.
// Returns 0 (unlimited) if not configured.
func (c *EinaiConfig) RateLimitRequestsPerMinute() int {
	return c.RateLimit.RequestsPerMinute
}

// RateLimitConcurrentSessions returns the configured concurrent sessions limit.
// Returns 0 (unlimited) if not configured.
func (c *EinaiConfig) RateLimitConcurrentSessions() int {
	return c.RateLimit.ConcurrentSessions
}

// DefaultConfigDir returns ~/.config/einai.
func DefaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "einai")
}

// DefaultDataDir returns ~/.einai.
func DefaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".einai")
}

// Load loads EinaiConfig from ~/.config/einai/config.toml.
// Returns default config if the file doesn't exist.
func Load() (*EinaiConfig, error) {
	path := filepath.Join(DefaultConfigDir(), "config.toml")
	cfg := &EinaiConfig{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.toml: %w", err)
	}
	return cfg, nil
}

// ExpandHome expands ~ in a path to the user home directory.
func ExpandHome(p string) string {
	return expandHome(p)
}

func expandHome(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
