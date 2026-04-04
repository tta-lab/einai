package session

import (
	"testing"

	"github.com/tta-lab/einai/internal/config"
	rt "github.com/tta-lab/einai/internal/runtime"
)

func TestResolveRuntime_FlagTakesPriority(t *testing.T) {
	cfg := &config.EinaiConfig{DefaultRuntime: "ei-native"}
	req := AgentRequest{Runtime: "claude-code"}

	resolved, err := resolveRuntime(req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != rt.ClaudeCode {
		t.Errorf("resolveRuntime() = %q, want %q (flag should beat config)", resolved, rt.ClaudeCode)
	}
}

func TestResolveRuntime_ConfigBeatsDefault(t *testing.T) {
	cfg := &config.EinaiConfig{DefaultRuntime: "ei-native"}
	req := AgentRequest{} // no flag

	resolved, err := resolveRuntime(req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != rt.EiNative {
		t.Errorf("resolveRuntime() = %q, want %q (config should beat builtin default)", resolved, rt.EiNative)
	}
}

func TestResolveRuntime_DefaultWhenNeitherSet(t *testing.T) {
	cfg := &config.EinaiConfig{} // no DefaultRuntime
	req := AgentRequest{}        // no flag

	resolved, err := resolveRuntime(req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != rt.ClaudeCode {
		t.Errorf("resolveRuntime() = %q, want %q (default is claude-code)", resolved, rt.ClaudeCode)
	}
}

func TestResolveRuntime_InvalidFlagReturnsError(t *testing.T) {
	cfg := &config.EinaiConfig{}
	req := AgentRequest{Runtime: "nonsense"}

	_, err := resolveRuntime(req, cfg)
	if err == nil {
		t.Error("resolveRuntime() expected error for invalid runtime, got nil")
	}
}

func TestResolveRuntime_InvalidConfigReturnsError(t *testing.T) {
	cfg := &config.EinaiConfig{DefaultRuntime: "bad-runtime"}
	req := AgentRequest{}

	_, err := resolveRuntime(req, cfg)
	if err == nil {
		t.Error("resolveRuntime() expected error for invalid config runtime, got nil")
	}
}
