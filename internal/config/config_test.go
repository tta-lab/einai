package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ReturnsDefaultsWhenFileDoesNotExist(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	// Default values should be returned by helper methods
	if cfg.AgentModel() != defaultModel {
		t.Errorf("AgentModel() = %q, want %q", cfg.AgentModel(), defaultModel)
	}
	if cfg.AgentMaxSteps() != defaultMaxSteps {
		t.Errorf("AgentMaxSteps() = %d, want %d", cfg.AgentMaxSteps(), defaultMaxSteps)
	}
	if cfg.AgentMaxTokens() != defaultMaxTokens {
		t.Errorf("AgentMaxTokens() = %d, want %d", cfg.AgentMaxTokens(), defaultMaxTokens)
	}
}

func TestAgentModel_ReturnsDefaultWhenEmpty(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.AgentModel(); got != defaultModel {
		t.Errorf("AgentModel() = %q, want default %q", got, defaultModel)
	}
}

func TestAgentModel_ReturnsConfiguredModel(t *testing.T) {
	cfg := &EinaiConfig{Model: "claude-opus-4"}
	if got := cfg.AgentModel(); got != "claude-opus-4" {
		t.Errorf("AgentModel() = %q, want %q", got, "claude-opus-4")
	}
}

func TestAgentMaxSteps_ReturnsDefaultWhenZero(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.AgentMaxSteps(); got != defaultMaxSteps {
		t.Errorf("AgentMaxSteps() = %d, want default %d", got, defaultMaxSteps)
	}
}

func TestAgentMaxSteps_ReturnsConfiguredValue(t *testing.T) {
	cfg := &EinaiConfig{MaxSteps: 200}
	if got := cfg.AgentMaxSteps(); got != 200 {
		t.Errorf("AgentMaxSteps() = %d, want %d", got, 200)
	}
}

func TestAgentMaxTokens_ReturnsDefaultWhenZero(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.AgentMaxTokens(); got != defaultMaxTokens {
		t.Errorf("AgentMaxTokens() = %d, want default %d", got, defaultMaxTokens)
	}
}

func TestAgentMaxTokens_ReturnsConfiguredValue(t *testing.T) {
	cfg := &EinaiConfig{MaxTokens: 65536}
	if got := cfg.AgentMaxTokens(); got != 65536 {
		t.Errorf("AgentMaxTokens() = %d, want %d", got, 65536)
	}
}

func TestRateLimitRequestsPerMinute_ReturnsZeroWhenNotSet(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.RateLimitRequestsPerMinute(); got != 0 {
		t.Errorf("RateLimitRequestsPerMinute() = %d, want 0", got)
	}
}

func TestRateLimitRequestsPerMinute_ReturnsConfiguredValue(t *testing.T) {
	cfg := &EinaiConfig{RateLimit: RateLimitConfig{RequestsPerMinute: 60}}
	if got := cfg.RateLimitRequestsPerMinute(); got != 60 {
		t.Errorf("RateLimitRequestsPerMinute() = %d, want 60", got)
	}
}

func TestRateLimitConcurrentSessions_ReturnsZeroWhenNotSet(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.RateLimitConcurrentSessions(); got != 0 {
		t.Errorf("RateLimitConcurrentSessions() = %d, want 0", got)
	}
}

func TestRateLimitConcurrentSessions_ReturnsConfiguredValue(t *testing.T) {
	cfg := &EinaiConfig{RateLimit: RateLimitConfig{ConcurrentSessions: 3}}
	if got := cfg.RateLimitConcurrentSessions(); got != 3 {
		t.Errorf("RateLimitConcurrentSessions() = %d, want 3", got)
	}
}

func TestExpandHome_WithTildePrefix(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{"~/path/to/file", filepath.Join(home, "path/to/file")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ExpandHome(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExpandHome_WithNoTildeReturnsUnchanged(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"/absolute/path"},
		{"relative/path"},
		{""},
		{"/home/user"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ExpandHome(tt.input)
			if got != tt.input {
				t.Errorf("ExpandHome(%q) = %q, want unchanged %q", tt.input, got, tt.input)
			}
		})
	}
}
