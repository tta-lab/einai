package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tta-lab/einai/internal/config"
)

// LenosAgentConfig holds execution config for lenos-runtime agents.
// Access controls CWD sandbox permissions: "ro" (read-only) or "rw" (read-write).
// Model is optional — when empty, the config.toml default is used at runtime.
type LenosAgentConfig struct {
	Model  string `yaml:"model"`
	Access string `yaml:"access"`
}

// Frontmatter holds parsed frontmatter from an agent .md file.
type Frontmatter struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Emoji       string                 `yaml:"emoji"`
	Color       string                 `yaml:"color"`
	ClaudeCode  map[string]interface{} `yaml:"claude-code"`
	Lenos       *LenosAgentConfig      `yaml:"lenos"`
}

// ParsedAgent holds the parsed frontmatter and body of an agent .md file.
type ParsedAgent struct {
	Frontmatter Frontmatter
	Body        string
}

// HasLenos returns true if the agent has a lenos: frontmatter block.
func (a *ParsedAgent) HasLenos() bool {
	return a.Frontmatter.Lenos != nil
}

// HasClaudeCode returns true if the agent has a claude-code: frontmatter block.
func (a *ParsedAgent) HasClaudeCode() bool {
	return a.Frontmatter.ClaudeCode != nil
}

// splitFrontmatter splits content into raw YAML frontmatter and body text.
func splitFrontmatter(content string) (yamlContent string, body string, err error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing opening --- delimiter")
	}

	rest := content[3:]
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("missing closing --- delimiter")
	}

	yamlContent = rest[:idx]
	body = rest[idx+4:]
	body = strings.TrimLeft(body, "\r\n")
	return yamlContent, body, nil
}

// ParseFile splits a canonical agent .md file into frontmatter and body.
func ParseFile(content string) (*ParsedAgent, error) {
	yamlContent, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if fm.Name == "" {
		return nil, fmt.Errorf("frontmatter missing required field: name")
	}

	return &ParsedAgent{
		Frontmatter: fm,
		Body:        body,
	}, nil
}

// Discover reads agent .md files from the configured paths and returns those
// with a lenos: OR claude-code: frontmatter block (or both).
func Discover(paths []string) ([]*ParsedAgent, error) {
	var agents []*ParsedAgent
	for _, rawPath := range paths {
		dir := config.ExpandHome(rawPath)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: agents path not found: %s\n", dir)
				continue
			}
			return nil, fmt.Errorf("reading agents dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
			}
			a, err := ParseFile(string(content))
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
				continue
			}
			// Include agents with either a lenos: block or claude-code: block (CC).
			if a.HasLenos() || a.HasClaudeCode() {
				agents = append(agents, a)
			}
		}
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Frontmatter.Name < agents[j].Frontmatter.Name
	})
	return agents, nil
}

// Find discovers agents from paths and returns the one matching name.
func Find(name string, paths []string) (*ParsedAgent, error) {
	agents, err := Discover(paths)
	if err != nil {
		return nil, fmt.Errorf("discover agents: %w", err)
	}
	for _, a := range agents {
		if a.Frontmatter.Name == name {
			return a, nil
		}
	}
	available := make([]string, len(agents))
	for i, a := range agents {
		available[i] = a.Frontmatter.Name
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("agent %q not found (no agents with lenos: frontmatter discovered)", name)
	}
	return nil, fmt.Errorf("agent %q not found — available: %s", name, strings.Join(available, ", "))
}

// ValidateAccess checks that the agent has a valid lenos: config block.
// Returns the access level ("ro" or "rw") on success.
func ValidateAccess(a *ParsedAgent, name string) (string, error) {
	if a.Frontmatter.Lenos == nil {
		return "", fmt.Errorf(
			"agent %q has no lenos: block — add 'lenos: access: ro' or 'lenos: access: rw'", name)
	}
	access := a.Frontmatter.Lenos.Access
	if access != "ro" && access != "rw" {
		return "", fmt.Errorf("agent %q has invalid access %q (want ro or rw)", name, access)
	}
	return access, nil
}

// ValidateRuntime checks that the agent supports the given runtime.
// For "claude-code" runtime: requires claude-code: block.
// For "lenos" runtime: requires lenos: block with valid access.
// Returns access level (for lenos) or "" (for claude-code) on success.
func ValidateRuntime(a *ParsedAgent, name, runtime string) (string, error) {
	switch runtime {
	case "claude-code":
		if !a.HasClaudeCode() {
			return "", fmt.Errorf("agent %q has no claude-code: block (required for claude-code runtime)", name)
		}
		return "", nil
	case "ei-native":
		return ValidateAccess(a, name)
	case "lenos":
		return ValidateAccess(a, name)
	default:
		return "", fmt.Errorf("unknown runtime %q", runtime)
	}
}
