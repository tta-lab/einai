package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SyncResult holds the result of a sync operation.
type SyncResult struct {
	Written []string // agent names written
	Skipped []string // agent names skipped (no claude-code block)
}

// Sync syncs agent files to a target directory, converting them to Claude Code format.
// It reads agent files from agentsPaths, parses them, and writes them to targetDir.
func Sync(agentsPaths []string, targetDir string, dryRun bool) (SyncResult, error) {
	result := SyncResult{}

	// Create target directory if it doesn't exist
	if !dryRun {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return result, fmt.Errorf("create target dir: %w", err)
		}
	}

	// Discover and process all agents
	for _, basePath := range agentsPaths {
		agents, err := Discover([]string{basePath})
		if err != nil {
			return result, fmt.Errorf("discover agents in %s: %w", basePath, err)
		}

		for _, agent := range agents {
			// Check if frontmatter has claude-code block
			if agent.Frontmatter.ClaudeCode == nil {
				result.Skipped = append(result.Skipped, agent.Frontmatter.Name)
				continue
			}

			// Build output content
			content := buildSyncContent(agent)

			// Write to target directory
			outputPath := filepath.Join(targetDir, agent.Frontmatter.Name+".md")
			if !dryRun {
				if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
					return result, fmt.Errorf("write %s: %w", outputPath, err)
				}
			}
			result.Written = append(result.Written, agent.Frontmatter.Name)
		}
	}

	return result, nil
}

// buildSyncContent constructs the Claude Code formatted agent file content.
func buildSyncContent(agent ParsedAgent) string {
	fm := agent.Frontmatter

	var sb strings.Builder

	// Write frontmatter header
	sb.WriteString("---\n")

	// Write name
	sb.WriteString(fmt.Sprintf("name: %s\n", fm.Name))

	// Write emoji
	if fm.Emoji != "" {
		sb.WriteString(fmt.Sprintf("emoji: %s\n", fm.Emoji))
	}

	// Write description (use yaml marshaling to handle special characters/multiline)
	if fm.Description != "" {
		if descYAML, err := yaml.Marshal(fm.Description); err == nil {
			sb.WriteString(fmt.Sprintf("description: %s", string(descYAML)))
		} else {
			sb.WriteString(fmt.Sprintf("description: %q\n", fm.Description))
		}
	}

	// Write color
	if fm.Color != "" {
		sb.WriteString(fmt.Sprintf("color: %s\n", fm.Color))
	}

	// Write model from claude-code block
	if model, ok := fm.ClaudeCode["model"].(string); ok {
		sb.WriteString(fmt.Sprintf("model: %s\n", model))
	}

	// Write tools from claude-code block
	if tools, ok := fm.ClaudeCode["tools"].([]interface{}); ok {
		sb.WriteString("tools: [")
		for i, t := range tools {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(t.(string))
		}
		sb.WriteString("]\n")
	}

	// Write frontmatter footer
	sb.WriteString("---\n")

	// Write body
	sb.WriteString(agent.Body)

	return sb.String()
}
