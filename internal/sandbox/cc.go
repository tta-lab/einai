package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/tta-lab/einai/internal/config"
)

// SyncResult holds the outcome of a SyncSandbox call.
type SyncResult struct {
	AllowWritePaths []string
	DenyReadPaths   []string
	GitDirCount     int
}

// SyncSandbox updates ~/.claude/settings.json with sandbox config from einai's sandbox.toml.
func SyncSandbox(settingsPath string, dryRun bool) (SyncResult, error) {
	sb, err := config.LoadSandboxWithError()
	if err != nil {
		return SyncResult{}, fmt.Errorf("loading sandbox.toml: %w", err)
	}
	if !sb.Enabled {
		return SyncResult{}, nil
	}

	allowWrite, gitDirCount := buildAllowWritePaths(sb)
	denyRead := sb.ExpandedDenyRead()

	result := SyncResult{
		AllowWritePaths: allowWrite,
		DenyReadPaths:   denyRead,
		GitDirCount:     gitDirCount,
	}

	if dryRun {
		return result, nil
	}

	settings, err := readOrInitSettings(settingsPath)
	if err != nil {
		return result, err
	}

	existingSockets := extractExistingSockets(settings)
	settings["sandbox"] = buildSandboxSection(sandboxSectionOpts{
		allowWrite:       allowWrite,
		denyWrite:        sb.ExpandedDenyWrite(),
		denyRead:         denyRead,
		allowRead:        sb.ExpandedAllowRead(),
		allowedDomains:   sb.Network.AllowedDomains,
		autoAllowBash:    sb.AutoAllowBashIfSandboxed,
		existingSockets:  existingSockets,
		excludedCommands: sb.ExcludedCommands,
	})

	perms, denySlice, err := extractPermsDenyList(settings)
	if err != nil {
		return result, err
	}
	denySlice = appendPermsDenyEntries(denySlice, sb.ExpandedPermissionsDeny())
	if denySlice == nil {
		denySlice = []interface{}{}
	}
	perms["deny"] = denySlice
	settings["permissions"] = perms

	if err := writeSettingsJSON(settingsPath, settings); err != nil {
		return result, fmt.Errorf("writing settings.json: %w", err)
	}
	return result, nil
}

func buildAllowWritePaths(sb *config.SandboxConfig) ([]string, int) {
	seen := make(map[string]bool)
	var paths []string

	for _, p := range sb.ExpandedAllowWrite() {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	gitDirCount := 0
	for _, gitDir := range CollectProjectGitDirs() {
		if !seen[gitDir] {
			seen[gitDir] = true
			paths = append(paths, gitDir)
			gitDirCount++
		}
	}

	sort.Strings(paths)
	return paths, gitDirCount
}

const daemonSocketPath = "~/.einai/daemon.sock"

func tmuxSocketPath() string {
	uid := os.Getuid()
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("/private/tmp/tmux-%d/default", uid)
	}
	return fmt.Sprintf("/tmp/tmux-%d/default", uid)
}

type sandboxSectionOpts struct {
	allowWrite       []string
	denyWrite        []string
	denyRead         []string
	allowRead        []string
	allowedDomains   []string
	autoAllowBash    *bool
	existingSockets  []string
	excludedCommands []string
}

func buildSandboxSection(opts sandboxSectionOpts) map[string]interface{} {
	toIfaceSlice := func(ss []string) []interface{} {
		out := make([]interface{}, len(ss))
		for i, s := range ss {
			out[i] = s
		}
		return out
	}

	daemonSock := expandHomePath(daemonSocketPath)
	tmuxSock := tmuxSocketPath()
	seen := map[string]bool{daemonSock: true, tmuxSock: true}
	sockets := []interface{}{daemonSock, tmuxSock}
	for _, s := range opts.existingSockets {
		if !seen[s] {
			seen[s] = true
			sockets = append(sockets, s)
		}
	}

	network := map[string]interface{}{
		"allowUnixSockets": sockets,
	}
	if len(opts.allowedDomains) > 0 {
		network["allowedDomains"] = toIfaceSlice(opts.allowedDomains)
	}

	fs := map[string]interface{}{
		"allowWrite": toIfaceSlice(opts.allowWrite),
	}
	if len(opts.denyRead) > 0 {
		fs["denyRead"] = toIfaceSlice(opts.denyRead)
	}
	if len(opts.denyWrite) > 0 {
		fs["denyWrite"] = toIfaceSlice(opts.denyWrite)
	}
	if len(opts.allowRead) > 0 {
		fs["allowRead"] = toIfaceSlice(opts.allowRead)
	}

	section := map[string]interface{}{
		"enabled":                  true,
		"failIfUnavailable":        true,
		"allowUnsandboxedCommands": false,
		"network":                  network,
		"filesystem":               fs,
	}
	if len(opts.excludedCommands) > 0 {
		section["excludedCommands"] = toIfaceSlice(opts.excludedCommands)
	}
	if opts.autoAllowBash != nil {
		section["autoAllowBashIfSandboxed"] = *opts.autoAllowBash
	}
	return section
}

func extractExistingSockets(settings map[string]interface{}) []string {
	sandbox, ok := settings["sandbox"].(map[string]interface{})
	if !ok {
		return nil
	}
	network, ok := sandbox["network"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := network["allowUnixSockets"].([]interface{})
	if !ok {
		return nil
	}
	sockets := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			sockets = append(sockets, s)
		}
	}
	return sockets
}

func appendPermsDenyEntries(denySlice []interface{}, entries []string) []interface{} {
	existing := make(map[string]struct{}, len(denySlice))
	for _, v := range denySlice {
		if s, ok := v.(string); ok {
			existing[s] = struct{}{}
		}
	}
	for _, e := range entries {
		if _, ok := existing[e]; !ok {
			denySlice = append(denySlice, e)
			existing[e] = struct{}{}
		}
	}
	return denySlice
}

func expandHomePath(p string) string {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
