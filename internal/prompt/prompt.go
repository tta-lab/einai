package prompt

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/tta-lab/einai/internal/command"
	"github.com/tta-lab/logos"
)

//go:embed ask_prompts/project.md
var projectPrompt string

//go:embed ask_prompts/repo.md
var repoPrompt string

//go:embed ask_prompts/url.md
var urlPrompt string

//go:embed ask_prompts/web.md
var webPrompt string

//go:embed ask_prompts/general.md
var generalPrompt string

// Mode identifies the ask operating mode.
type Mode string

const (
	ModeProject Mode = "project"
	ModeRepo    Mode = "repo"
	ModeURL     Mode = "url"
	ModeWeb     Mode = "web"
	ModeGeneral Mode = "general"
)

// Valid reports whether m is a known ask mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeProject, ModeRepo, ModeURL, ModeWeb, ModeGeneral:
		return true
	}
	return false
}

// ModeParams holds mode-specific parameters for prompt building.
type ModeParams struct {
	WorkingDir    string
	ProjectPath   string
	RepoLocalPath string
	RawURL        string
	Question      string
}

// BuildSystemPromptForMode constructs the full system prompt for the given mode.
func BuildSystemPromptForMode(mode Mode, params ModeParams) (string, []logos.CommandDoc, error) {
	promptData := logos.PromptData{
		WorkingDir: params.WorkingDir,
		Platform:   runtime.GOOS,
		Date:       time.Now().Format("2006-01-02"),
	}

	var extra string
	var cmds []logos.CommandDoc

	switch mode {
	case ModeProject:
		if params.ProjectPath == "" {
			return "", nil, fmt.Errorf("project path required for project mode")
		}
		extra = strings.ReplaceAll(projectPrompt, "{projectPath}", params.ProjectPath)
		cmds = command.AllCommands()
		promptData.WorkingDir = params.ProjectPath
		promptData.Commands = cmds
	case ModeRepo:
		if params.RepoLocalPath == "" {
			return "", nil, fmt.Errorf("repo local path required for repo mode")
		}
		extra = strings.ReplaceAll(repoPrompt, "{localPath}", params.RepoLocalPath)
		cmds = command.AllCommands()
		promptData.WorkingDir = params.RepoLocalPath
		promptData.Commands = cmds
	case ModeURL:
		extra = strings.ReplaceAll(urlPrompt, "{rawURL}", params.RawURL)
		cmds = command.NetworkCommands()
		promptData.Commands = cmds
	case ModeWeb:
		extra = strings.ReplaceAll(webPrompt, "{query}", params.Question)
		cmds = command.NetworkCommands()
		promptData.Commands = cmds
	case ModeGeneral:
		extra = strings.ReplaceAll(generalPrompt, "{cwd}", params.WorkingDir)
		cmds = command.AllCommands()
		promptData.Commands = cmds
	default:
		return "", nil, fmt.Errorf("unknown ask mode: %s", mode)
	}

	systemPrompt, err := logos.BuildSystemPrompt(promptData)
	if err != nil {
		return "", nil, fmt.Errorf("build system prompt: %w", err)
	}
	if extra != "" {
		systemPrompt += "\n\n" + extra
	}
	return systemPrompt, cmds, nil
}
