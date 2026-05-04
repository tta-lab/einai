package config

import (
	"os"
	"path/filepath"
	"reflect"
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
agents_paths = ["/custom/agents"]
default_runtime = "lenos"
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
	wantPaths := []string{"/custom/agents"}
	if !reflect.DeepEqual(cfg.AgentsPaths, wantPaths) {
		t.Errorf("AgentsPaths = %v, want %v", cfg.AgentsPaths, wantPaths)
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
agents_paths = ["/agents/a", "/agents/b"]
default_runtime = "lenos"
max_run_timeout = 3600

[jobqueue]
max_parallel = 6
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
	wantPaths := []string{"/agents/a", "/agents/b"}
	if !reflect.DeepEqual(cfg.AgentsPaths, wantPaths) {
		t.Errorf("AgentsPaths = %v, want %v", cfg.AgentsPaths, wantPaths)
	}
	if cfg.DefaultRuntime != "lenos" {
		t.Errorf("DefaultRuntime = %q, want lenos", cfg.DefaultRuntime)
	}
	if cfg.MaxRunTimeout != 3600 {
		t.Errorf("MaxRunTimeout = %d, want 3600", cfg.MaxRunTimeout)
	}
	if cfg.Jobqueue.MaxParallel != 6 {
		t.Errorf("Jobqueue.MaxParallel = %d, want 6", cfg.Jobqueue.MaxParallel)
	}
	if cfg.MaxParallel() != 6 {
		t.Errorf("MaxParallel() = %d, want 6", cfg.MaxParallel())
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

func TestMaxParallel_DefaultsToDefault(t *testing.T) {
	cfg := &EinaiConfig{}
	if cfg.MaxParallel() != 4 {
		t.Errorf("MaxParallel() = %d, want 4", cfg.MaxParallel())
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
