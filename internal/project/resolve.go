package project

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// Project holds a ttal project entry.
type Project struct {
	Alias string `json:"alias"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

// List calls ttal project list --json and returns all active projects.
func List() ([]Project, error) {
	out, err := exec.Command("ttal", "project", "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("ttal project list: %w", err)
	}
	var projects []Project
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, fmt.Errorf("parse ttal project list output: %w", err)
	}
	return projects, nil
}

// GetProjectPath resolves a project alias to its filesystem path.
func GetProjectPath(alias string) (string, error) {
	projects, err := List()
	if err != nil {
		return "", err
	}
	for _, p := range projects {
		if p.Alias == alias {
			if p.Path == "" {
				return "", fmt.Errorf("project %q exists but has no path configured", alias)
			}
			return p.Path, nil
		}
	}
	return "", fmt.Errorf("project %q not found", alias)
}

