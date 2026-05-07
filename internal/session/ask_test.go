package session

import (
	"strings"
	"testing"
)

func TestBuildAskArgs_AlwaysIncludesReadonly(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		question string
		cwd      string
		ctxFile  string
	}{
		{"general", ModeGeneral, "hi", "/tmp/x", "/tmp/ctx.md"},
		{"project", ModeProject, "what is this?", "/p", "/tmp/ctx.md"},
		{"repo", ModeRepo, "explain", "/r", "/tmp/ctx.md"},
		{"url", ModeURL, "summarize", "/u", "/tmp/ctx.md"},
		{"web", ModeWeb, "search golang", "/w", "/tmp/ctx.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := AskRequest{Mode: tt.mode, Question: tt.question, WorkingDir: tt.cwd}
			args := buildAskArgs(req, tt.cwd, tt.ctxFile)
			got := strings.Join(args, " ")
			if !strings.Contains(got, "--readonly") {
				t.Errorf("expected --readonly in %q", got)
			}
			if !strings.Contains(got, "--small-model") {
				t.Errorf("expected --small-model in %q", got)
			}
			if !strings.Contains(got, "-f "+tt.ctxFile) {
				t.Errorf("expected -f %s in %q", tt.ctxFile, got)
			}
		})
	}
}

func TestBuildAskArgs_AgentNameMatchesMode(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeGeneral, "ask-general"},
		{ModeProject, "ask-project"},
		{ModeRepo, "ask-repo"},
		{ModeURL, "ask-url"},
		{ModeWeb, "ask-web"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			req := AskRequest{Mode: tt.mode, Question: "hi", WorkingDir: "/x"}
			args := buildAskArgs(req, "/x", "/tmp/ctx.md")
			got := strings.Join(args, " ")
			if !strings.Contains(got, "--agent "+tt.want) {
				t.Errorf("expected --agent %s in %q", tt.want, got)
			}
			if !strings.Contains(got, "--small-model") {
				t.Errorf("expected --small-model in %q", got)
			}
		})
	}
}

func TestBuildAskArgs_EmptyQuestionNoSeparator(t *testing.T) {
	req := AskRequest{Mode: ModeGeneral, Question: "", WorkingDir: "/x"}
	args := buildAskArgs(req, "/x", "/tmp/ctx.md")
	got := strings.Join(args, " ")
	if strings.Contains(got, " -- ") {
		t.Errorf("expected no -- separator for empty question, got %q", got)
	}
	if !strings.Contains(got, "--readonly") {
		t.Errorf("expected --readonly even with empty question, got %q", got)
	}
	if !strings.Contains(got, "--small-model") {
		t.Errorf("expected --small-model in %q", got)
	}
}
