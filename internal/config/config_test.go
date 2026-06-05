package config

import (
	"os"
	"path/filepath"
	"testing"

	rt "github.com/tta-lab/einai/internal/runtime"
)

func TestLoad_ReturnsDefaultsWhenFileDoesNotExist(t *testing.T) {
	cfg, err := LoadFromPath(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("LoadFromPath() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFromPath() returned nil config")
	}
	if cfg.ReferencesPath != "" {
		t.Errorf("ReferencesPath = %q, want empty string", cfg.ReferencesPath)
	}
	if cfg.AgentDefaultRuntime() != string(rt.Default) {
		t.Errorf("AgentDefaultRuntime() = %q, want %q", cfg.AgentDefaultRuntime(), string(rt.Default))
	}
}

func TestLoadFromPath_ParsesTOMLCorrectly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	tomlContent := `references_path = "/custom/references"
default_runtime = "lenos"
model = "claude-sonnet-4-6"
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	cfg, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath() error: %v", err)
	}
	if cfg.ReferencesPath != "/custom/references" {
		t.Errorf("ReferencesPath = %q, want /custom/references", cfg.ReferencesPath)
	}
	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", cfg.Model)
	}
}

func TestAgentDefaultRuntime_UsesDefaultWhenUnset(t *testing.T) {
	cfg := &EinaiConfig{}
	if cfg.AgentDefaultRuntime() != string(rt.Default) {
		t.Errorf("AgentDefaultRuntime() = %q, want %q", cfg.AgentDefaultRuntime(), string(rt.Default))
	}
}

func TestAgentDefaultRuntime_UsesConfigWhenSet(t *testing.T) {
	cfg := &EinaiConfig{DefaultRuntime: "claude-code"}
	if cfg.AgentDefaultRuntime() != "claude-code" {
		t.Errorf("AgentDefaultRuntime() = %q, want %q", cfg.AgentDefaultRuntime(), "claude-code")
	}
}

func TestLoadConfig_FullConfigParsing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	tomlContent := `references_path = "/custom/refs"
default_runtime = "lenos"
max_run_timeout = 3600
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	cfg, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath() error: %v", err)
	}
	if cfg.ReferencesPath != "/custom/refs" {
		t.Errorf("ReferencesPath = %q, want /custom/refs", cfg.ReferencesPath)
	}
	if cfg.DefaultRuntime != "lenos" {
		t.Errorf("DefaultRuntime = %q, want lenos", cfg.DefaultRuntime)
	}
	if cfg.MaxRunTimeout != 3600 {
		t.Errorf("MaxRunTimeout = %d, want 3600", cfg.MaxRunTimeout)
	}
}

func TestLoadFromPath_ReturnsEmptyConfigForMissingFile(t *testing.T) {
	cfg, err := LoadFromPath("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("LoadFromPath() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFromPath() returned nil")
	}
}

func TestAgentReferencesPath_DefaultsToXDG(t *testing.T) {
	cfg := &EinaiConfig{}
	path := cfg.AgentReferencesPath()
	if path == "" {
		t.Errorf("AgentReferencesPath() returned empty")
	}
}

func TestDefaultDataDir_UsesOverride(t *testing.T) {
	tempDir := t.TempDir()
	SetTestDataDir(tempDir)
	t.Cleanup(ClearTestDataDir)

	if DefaultDataDir() != tempDir {
		t.Errorf("DefaultDataDir() = %q, want %q", DefaultDataDir(), tempDir)
	}
}
