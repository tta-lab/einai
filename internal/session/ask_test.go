package session

import (
	"strings"
	"testing"
)

func TestBuildAskArgs_GeneralModeIncludesReadonly(t *testing.T) {
	req := AskRequest{Mode: ModeGeneral, Question: "hi", WorkingDir: "/tmp/x"}
	args := buildAskArgs(req, "/tmp/x", "/tmp/ctx.md")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--readonly") {
		t.Errorf("expected --readonly, got %q", got)
	}
	if !strings.Contains(got, "--agent ask-general") {
		t.Errorf("expected --agent ask-general, got %q", got)
	}
	if !strings.Contains(got, "-f /tmp/ctx.md") {
		t.Errorf("expected -f /tmp/ctx.md, got %q", got)
	}
	if !strings.Contains(got, "-- hi") {
		t.Errorf("expected -- hi, got %q", got)
	}
}

func TestBuildAskArgs_ProjectModeIncludesReadonly(t *testing.T) {
	req := AskRequest{Mode: ModeProject, Question: "what is this?", WorkingDir: "/p"}
	args := buildAskArgs(req, "/p", "/tmp/ctx.md")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--readonly") {
		t.Errorf("expected --readonly, got %q", got)
	}
	if !strings.Contains(got, "--agent ask-project") {
		t.Errorf("expected --agent ask-project, got %q", got)
	}
}

func TestBuildAskArgs_RepoModeIncludesReadonly(t *testing.T) {
	req := AskRequest{Mode: ModeRepo, Question: "explain", WorkingDir: "/r"}
	args := buildAskArgs(req, "/r", "/tmp/ctx.md")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--readonly") {
		t.Errorf("expected --readonly, got %q", got)
	}
	if !strings.Contains(got, "--agent ask-repo") {
		t.Errorf("expected --agent ask-repo, got %q", got)
	}
}

func TestBuildAskArgs_URLModeIncludesReadonly(t *testing.T) {
	req := AskRequest{Mode: ModeURL, Question: "summarize", WorkingDir: "/u"}
	args := buildAskArgs(req, "/u", "/tmp/ctx.md")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--readonly") {
		t.Errorf("expected --readonly, got %q", got)
	}
	if !strings.Contains(got, "--agent ask-url") {
		t.Errorf("expected --agent ask-url, got %q", got)
	}
}

func TestBuildAskArgs_WebModeIncludesReadonly(t *testing.T) {
	req := AskRequest{Mode: ModeWeb, Question: "search golang", WorkingDir: "/w"}
	args := buildAskArgs(req, "/w", "/tmp/ctx.md")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--readonly") {
		t.Errorf("expected --readonly, got %q", got)
	}
	if !strings.Contains(got, "--agent ask-web") {
		t.Errorf("expected --agent ask-web, got %q", got)
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
}
