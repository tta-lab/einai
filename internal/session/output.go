package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tta-lab/einai/internal/config"
)

// outputDir returns the directory for output files: ~/.einai/outputs/<runtime>/
func outputDir(runtime string) string {
	return filepath.Join(config.DefaultDataDir(), "outputs", runtime)
}

// WriteOutputFile writes the agent result to ~/.einai/outputs/<runtime>/<stem>.md.
func WriteOutputFile(result, runtime, stem string) error {
	dir := outputDir(runtime)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	path := filepath.Join(dir, stem+".md")
	if err := os.WriteFile(path, []byte(result), 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

// ReadOutputFile reads the agent result from ~/.einai/outputs/<runtime>/<stem>.md.
func ReadOutputFile(runtime, stem string) (string, error) {
	path := filepath.Join(outputDir(runtime), stem+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}
	return string(data), nil
}
