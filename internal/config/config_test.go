package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_ReturnsDefaultsWhenFileDoesNotExist(t *testing.T) {
	cfg, err := LoadFromPath(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("LoadFromPath() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFromPath() returned nil config")
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
func TestPueueGroup_ReturnsDefaultWhenEmpty(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.PueueGroup(); got != defaultPueueGroup {
		t.Errorf("PueueGroup() = %q, want default %q", got, defaultPueueGroup)
	}
}

func TestPueueGroup_ReturnsConfiguredValue(t *testing.T) {
	cfg := &EinaiConfig{Pueue: PueueConfig{Group: "myagents"}}
	if got := cfg.PueueGroup(); got != "myagents" {
		t.Errorf("PueueGroup() = %q, want %q", got, "myagents")
	}
}

func TestPueueParallel_ReturnsDefaultWhenZero(t *testing.T) {
	cfg := &EinaiConfig{}
	if got := cfg.PueueParallel(); got != defaultPueueParallel {
		t.Errorf("PueueParallel() = %d, want default %d", got, defaultPueueParallel)
	}
}

func TestPueueParallel_ReturnsConfiguredValue(t *testing.T) {
	cfg := &EinaiConfig{Pueue: PueueConfig{Parallel: 5}}
	if got := cfg.PueueParallel(); got != 5 {
		t.Errorf("PueueParallel() = %d, want 5", got)
	}
}

func TestLoadFromPath_ParsesPueueSection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	tomlContent := `[pueue]
group = "workers"
parallel = 4
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	cfg, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath() error: %v", err)
	}
	if cfg.Pueue.Group != "workers" {
		t.Errorf("Pueue.Group = %q, want workers", cfg.Pueue.Group)
	}
	if cfg.Pueue.Parallel != 4 {
		t.Errorf("Pueue.Parallel = %d, want 4", cfg.Pueue.Parallel)
	}
	if cfg.PueueGroup() != "workers" {
		t.Errorf("PueueGroup() = %q, want workers", cfg.PueueGroup())
	}
	if cfg.PueueParallel() != 4 {
		t.Errorf("PueueParallel() = %d, want 4", cfg.PueueParallel())
	}
}

func TestLoadFromPath_ParsesTOMLCorrectly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	tomlContent := `model = "claude-opus-4"
max_steps = 50
max_tokens = 65536
references_path = "/custom/references"
agents_paths = ["/custom/agents"]
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
	if cfg.Model != "claude-opus-4" {
		t.Errorf("Model = %q, want claude-opus-4", cfg.Model)
	}
	if cfg.MaxSteps != 50 {
		t.Errorf("MaxSteps = %d, want 50", cfg.MaxSteps)
	}
	if cfg.MaxTokens != 65536 {
		t.Errorf("MaxTokens = %d, want 65536", cfg.MaxTokens)
	}
}
