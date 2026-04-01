package session

import "github.com/tta-lab/einai/internal/project"

// resolveProjectPath is a thin wrapper so agent.go doesn't import project directly.
func resolveProjectPath(alias string) (string, error) {
	return project.GetProjectPath(alias)
}
