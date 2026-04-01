package agent

import (
	"testing"
)

func TestParseFile_WithValidFrontmatterAndBody(t *testing.T) {
	content := `---
name: test-agent
description: A test agent
emoji: "🤖"
color: blue
---

This is the body content.
It has multiple lines.
`

	agent, err := ParseFile(content)
	if err != nil {
		t.Fatalf("ParseFile() returned unexpected error: %v", err)
	}
	if agent == nil {
		t.Fatal("ParseFile() returned nil agent")
	}
}

func TestParseFile_ReturnsErrorWhenFrontmatterMissingOpening(t *testing.T) {
	content := `name: test-agent
---
body content
`

	_, err := ParseFile(content)
	if err == nil {
		t.Error("ParseFile() expected error for missing opening ---, got nil")
	}
}

func TestParseFile_ReturnsErrorWhenMissingClosing(t *testing.T) {
	content := `---
name: test-agent
body content
`

	_, err := ParseFile(content)
	if err == nil {
		t.Error("ParseFile() expected error for missing closing ---, got nil")
	}
}

func TestParseFile_ReturnsErrorWhenNameFieldIsEmpty(t *testing.T) {
	content := `---
name:
description: No name provided
---

body content
`

	_, err := ParseFile(content)
	if err == nil {
		t.Error("ParseFile() expected error for empty name, got nil")
	}
}

func TestParseFile_ParsedFrontmatterFieldsMatchExpected(t *testing.T) {
	content := `---
name: my-agent
description: My test agent
emoji: "🦊"
color: orange
ttal:
  access: rw
---

This is the body.
`

	agent, err := ParseFile(content)
	if err != nil {
		t.Fatalf("ParseFile() returned unexpected error: %v", err)
	}

	if agent.Frontmatter.Name != "my-agent" {
		t.Errorf("Name = %q, want %q", agent.Frontmatter.Name, "my-agent")
	}
	if agent.Frontmatter.Description != "My test agent" {
		t.Errorf("Description = %q, want %q", agent.Frontmatter.Description, "My test agent")
	}
	if agent.Frontmatter.Emoji != "🦊" {
		t.Errorf("Emoji = %q, want %q", agent.Frontmatter.Emoji, "🦊")
	}
	if agent.Frontmatter.Color != "orange" {
		t.Errorf("Color = %q, want %q", agent.Frontmatter.Color, "orange")
	}
}

func TestParseFile_BodyIsCorrectlyExtracted(t *testing.T) {
	bodyContent := `This is the body.
It has multiple lines.
And more content here.`

	content := `---
name: body-test
description: Testing body extraction
---

` + bodyContent

	agent, err := ParseFile(content)
	if err != nil {
		t.Fatalf("ParseFile() returned unexpected error: %v", err)
	}

	if agent.Body != bodyContent {
		t.Errorf("Body = %q, want %q", agent.Body, bodyContent)
	}
}

func TestParseFile_ParsesClaudeCodeBlock(t *testing.T) {
	content := `---
name: claude-agent
description: Agent with claude-code block
emoji: "🎯"
color: purple
claude-code:
  model: claude-sonnet-4-6
  tools:
    - read
    - write
    - bash
---

Agent body content.
`

	agent, err := ParseFile(content)
	if err != nil {
		t.Fatalf("ParseFile() returned unexpected error: %v", err)
	}

	if agent.Frontmatter.ClaudeCode == nil {
		t.Fatal("ClaudeCode block is nil")
	}
	if agent.Frontmatter.ClaudeCode["model"] != "claude-sonnet-4-6" {
		t.Errorf("ClaudeCode[model] = %v, want %q", agent.Frontmatter.ClaudeCode["model"], "claude-sonnet-4-6")
	}
}

func TestParseFile_WithTtalBlock(t *testing.T) {
	content := `---
name: ttal-agent
description: Agent with ttal block
ttal:
  access: ro
  model: claude-opus-4
---

Agent body.
`

	agent, err := ParseFile(content)
	if err != nil {
		t.Fatalf("ParseFile() returned unexpected error: %v", err)
	}

	if agent.Frontmatter.Ttal == nil {
		t.Fatal("Ttal block is nil")
	}
	if agent.Frontmatter.Ttal.Access != "ro" {
		t.Errorf("Ttal.Access = %q, want %q", agent.Frontmatter.Ttal.Access, "ro")
	}
	if agent.Frontmatter.Ttal.Model != "claude-opus-4" {
		t.Errorf("Ttal.Model = %q, want %q", agent.Frontmatter.Ttal.Model, "claude-opus-4")
	}
}

func TestSplitFrontmatter_VariousFormats(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantYamlErr bool
		wantBodyErr bool
	}{
		{
			name:        "no delimiters",
			content:     "name: test\n---\nbody",
			wantYamlErr: true,
		},
		{
			name:        "no closing delimiter",
			content:     "---\nname: test\nbody",
			wantYamlErr: false,
			wantBodyErr: true,
		},
		{
			name:        "empty yaml with body",
			content:     "---\n---\nbody content",
			wantYamlErr: false,
			wantBodyErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := splitFrontmatter(tt.content)
			if tt.wantYamlErr || tt.wantBodyErr {
				if err == nil {
					t.Errorf("splitFrontmatter() expected error, got nil")
				}
			}
		})
	}
}
