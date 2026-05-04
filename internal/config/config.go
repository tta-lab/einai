package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	rt "github.com/tta-lab/einai/internal/runtime"
)

const (
	defaultMaxParallel = 4
)

// JobqueueConfig holds job queue configuration.
type JobqueueConfig struct {
	// MaxParallel is the maximum concurrent jobs (default: 4).
	MaxParallel int `toml:"max_parallel"`
}

// EinaiConfig holds einai daemon configuration loaded from ~/.config/einai/config.toml.
type EinaiConfig struct {
	// Local path for cloned OSS reference repos (default: ~/.einai/references/)
	ReferencesPath string `toml:"references_path"`
	// Paths to search for agent .md files
	AgentsPaths []string `toml:"agents_paths"`
	// Default runtime for agent execution: "lenos" or "claude-code" (default: "lenos")
	DefaultRuntime string `toml:"default_runtime"`
	// Maximum run timeout in seconds for agent/run and ask requests (default: 1200 = 20min)
	MaxRunTimeout int `toml:"max_run_timeout"`
	// Job queue configuration
	Jobqueue JobqueueConfig `toml:"jobqueue"`
}

// AgentMaxRunTimeout returns the configured max run timeout as a duration.
// Default is 1200s (20 minutes).
func (c *EinaiConfig) AgentMaxRunTimeout() time.Duration {
	if c.MaxRunTimeout > 0 {
		return time.Duration(c.MaxRunTimeout) * time.Second
	}
	return 1200 * time.Second
}

// AgentDefaultRuntime returns the configured default runtime or the built-in default.
// Returns the raw string — callers should call runtime.Parse() to validate.
func (c *EinaiConfig) AgentDefaultRuntime() string {
	if c.DefaultRuntime != "" {
		return c.DefaultRuntime
	}
	return string(rt.Default)
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

// MaxParallel returns the configured job queue parallelism or the default 4.
func (c *EinaiConfig) MaxParallel() int {
	if c.Jobqueue.MaxParallel > 0 {
		return c.Jobqueue.MaxParallel
	}
	return defaultMaxParallel
}

// DefaultConfigDir returns ~/.config/einai.
func DefaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "einai")
}

// testDataDir is set by tests to override DefaultDataDir().
var testDataDir string

// SetTestDataDir sets the data directory for testing.
// Callers should call ClearTestDataDir in their test cleanup.
func SetTestDataDir(dir string) {
	testDataDir = dir
}

// ClearTestDataDir clears the test data directory override.
func ClearTestDataDir() {
	testDataDir = ""
}

// DefaultDataDir returns ~/.einai (or the test override if set).
func DefaultDataDir() string {
	if testDataDir != "" {
		return testDataDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".einai")
}

// LoadFromPath loads EinaiConfig from the specified path.
// Returns an empty config if the file doesn't exist. Use accessor methods
// to get defaults for missing values.
func LoadFromPath(path string) (*EinaiConfig, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &EinaiConfig{}, nil
		}
		return nil, fmt.Errorf("stat config: %w", err)
	}

	var cfg EinaiConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return &cfg, nil
}

// Load loads the EinaiConfig from the default config path.
func Load() (*EinaiConfig, error) {
	return LoadFromPath(filepath.Join(DefaultConfigDir(), "config.toml"))
}

// ExpandHome expands a leading ~ in the given path to the user's home directory.
func ExpandHome(path string) string {
	return expandHome(path)
}

// expandHome expands a leading ~ in the given path to the user's home directory.
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if len(path) == 1 {
		return home
	}

	if path[1] == '/' {
		return filepath.Join(home, path[2:])
	}

	return path
}

// TaskrcPath returns the path to the user's .taskrc file.
func TaskrcPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".taskrc")
}
