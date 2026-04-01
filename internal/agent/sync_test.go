package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestAgentFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func TestSync_DryRunWritesNothingButReturnsWrittenList(t *testing.T) {
	// Create temp directory with agent files
	tmpDir, err := os.MkdirTemp("", "sync-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create agent with claude-code block (should be synced)
	createTestAgentFile(t, tmpDir, "test-agent", `---
name: test-agent
description: A test agent
emoji: "🤖"
color: blue
claude-code:
  model: claude-sonnet-4-6
  tools:
    - read
    - write
---

Agent body content.
`)

	// Create target directory
	targetDir := filepath.Join(tmpDir, "target")

	// Sync with dryRun=true
	result, err := Sync([]string{tmpDir}, targetDir, true)
	if err != nil {
		t.Fatalf("Sync() returned unexpected error: %v", err)
	}

	// Should report written
	if len(result.Written) != 1 || result.Written[0] != "test-agent" {
		t.Errorf("Written = %v, want [test-agent]", result.Written)
	}

	// File should NOT be written
	if _, err := os.Stat(filepath.Join(targetDir, "test-agent.md")); !os.IsNotExist(err) {
		t.Error("File should not exist with dryRun=true")
	}
}

func TestSync_DryRunFalseWritesFilesToTargetDir(t *testing.T) {
	// Create temp directory with agent files
	tmpDir, err := os.MkdirTemp("", "sync-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create agent with claude-code block
	createTestAgentFile(t, tmpDir, "write-agent", `---
name: write-agent
description: An agent to write
emoji: "✍️"
claude-code:
  model: claude-opus-4-5
  tools:
    - read
    - write
---

Write body.
`)

	// Create target directory
	targetDir := filepath.Join(tmpDir, "output")

	// Sync with dryRun=false
	result, err := Sync([]string{tmpDir}, targetDir, false)
	if err != nil {
		t.Fatalf("Sync() returned unexpected error: %v", err)
	}

	if len(result.Written) != 1 || result.Written[0] != "write-agent" {
		t.Errorf("Written = %v, want [write-agent]", result.Written)
	}

	// File should be written
	expectedPath := filepath.Join(targetDir, "write-agent.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Error("File should exist with dryRun=false")
	}
}

func TestSync_AgentWithNoClaudeCodeBlockIsSkipped(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sync-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create agent WITHOUT claude-code block (should be skipped)
	createTestAgentFile(t, tmpDir, "skip-agent", `---
name: skip-agent
description: An agent without claude-code
emoji: "⏭️"
ttal:
  access: ro
---

Skip body.
`)

	targetDir := filepath.Join(tmpDir, "target")

	result, err := Sync([]string{tmpDir}, targetDir, false)
	if err != nil {
		t.Fatalf("Sync() returned unexpected error: %v", err)
	}

	// Should be skipped
	if len(result.Skipped) != 1 || result.Skipped[0] != "skip-agent" {
		t.Errorf("Skipped = %v, want [skip-agent]", result.Skipped)
	}

	// Should not be written
	if len(result.Written) != 0 {
		t.Errorf("Written = %v, want []", result.Written)
	}
}

func TestBuildSyncContent_ProducesFrontmatterWithNameEmojiDescription(t *testing.T) {
	agent := ParsedAgent{
		Frontmatter: Frontmatter{
			Name:        "my-agent",
			Description: "My test agent description",
			Emoji:       "🚀",
			Color:       "green",
			ClaudeCode: map[string]interface{}{
				"model": "claude-sonnet-4-6",
				"tools": []interface{}{"read", "write"},
			},
		},
		Body: "Agent body content.",
	}

	content := buildSyncContent(agent)

	// Check frontmatter header
	if !strings.Contains(content, "---") {
		t.Error("Content should contain frontmatter header")
	}

	// Check name
	if !strings.Contains(content, "name: my-agent") {
		t.Errorf("Content should contain name, got:\n%s", content)
	}

	// Check emoji
	if !strings.Contains(content, "emoji: 🚀") {
		t.Errorf("Content should contain emoji, got:\n%s", content)
	}

	// Check description
	if !strings.Contains(content, "description:") {
		t.Errorf("Content should contain description, got:\n%s", content)
	}

	// Check model from claude-code
	if !strings.Contains(content, "model: claude-sonnet-4-6") {
		t.Errorf("Content should contain model from claude-code, got:\n%s", content)
	}

	// Check tools from claude-code
	if !strings.Contains(content, "tools: [read, write]") {
		t.Errorf("Content should contain tools, got:\n%s", content)
	}
}

func TestBuildSyncContent_ExcludesTtalBlock(t *testing.T) {
	agent := ParsedAgent{
		Frontmatter: Frontmatter{
			Name:        "no-ttal-agent",
			Description: "Test agent",
			Ttal: &EinaiAgentConfig{
				Access: "rw",
				Model:  "claude-opus-4",
			},
			ClaudeCode: map[string]interface{}{
				"model": "claude-sonnet-4-6",
				"tools": []interface{}{"read"},
			},
		},
		Body: "Body",
	}

	content := buildSyncContent(agent)

	// ttal block should not be in output
	if strings.Contains(content, "ttal:") {
		t.Errorf("Content should not contain ttal block, got:\n%s", content)
	}

	// claude-code block fields should be included
	if !strings.Contains(content, "model:") {
		t.Error("Content should contain model from claude-code")
	}
}

func TestBuildSyncContent_UsesClaudeCodeModelAndTools(t *testing.T) {
	agent := ParsedAgent{
		Frontmatter: Frontmatter{
			Name: "cc-agent",
			ClaudeCode: map[string]interface{}{
				"model": "claude-opus-4-5",
				"tools": []interface{}{"read", "write", "bash", "edit"},
			},
		},
		Body: "Body content",
	}

	content := buildSyncContent(agent)

	// Model should come from claude-code block
	if !strings.Contains(content, "model: claude-opus-4-5") {
		t.Errorf("Content should contain claude-code model, got:\n%s", content)
	}

	// Tools should come from claude-code block
	if !strings.Contains(content, "tools: [read, write, bash, edit]") {
		t.Errorf("Content should contain claude-code tools, got:\n%s", content)
	}
}

func TestBuildSyncContent_BodyIsAppended(t *testing.T) {
	body := "This is the agent body.\nWith multiple lines."
	agent := ParsedAgent{
		Frontmatter: Frontmatter{
			Name: "body-agent",
			ClaudeCode: map[string]interface{}{
				"model": "claude-sonnet-4-6",
			},
		},
		Body: body,
	}

	content := buildSyncContent(agent)

	// Body should be at the end
	if !strings.HasSuffix(content, body) {
		t.Errorf("Content should end with body, got:\n%s", content)
	}
}

func TestSync_MultipleAgents(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sync-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple agents
	createTestAgentFile(t, tmpDir, "agent1", `---
name: agent1
description: First agent
claude-code:
  model: claude-sonnet-4-6
  tools:
    - read
---

Body 1.
`)

	createTestAgentFile(t, tmpDir, "agent2", `---
name: agent2
description: Second agent
claude-code:
  model: claude-opus-4
  tools:
    - read
    - write
---

Body 2.
`)

	// Agent without claude-code block
	createTestAgentFile(t, tmpDir, "agent3", `---
name: agent3
description: Third agent (no claude-code)
ttal:
  access: ro
---

Body 3.
`)

	targetDir := filepath.Join(tmpDir, "target")

	result, err := Sync([]string{tmpDir}, targetDir, false)
	if err != nil {
		t.Fatalf("Sync() returned unexpected error: %v", err)
	}

	// Should write 2 agents
	if len(result.Written) != 2 {
		t.Errorf("Written = %v, want 2 agents", result.Written)
	}

	// Should skip 1 agent
	if len(result.Skipped) != 1 || result.Skipped[0] != "agent3" {
		t.Errorf("Skipped = %v, want [agent3]", result.Skipped)
	}

	// Both files should exist
	for _, name := range []string{"agent1", "agent2"} {
		path := filepath.Join(targetDir, name+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected %s.md to exist", name)
		}
	}
}
