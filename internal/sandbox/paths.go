package sandbox

import (
	"path/filepath"

	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/project"
	"github.com/tta-lab/logos"
)

// BuildAgentSandboxPaths constructs AllowedPaths from sandbox config + CWD + project git dirs.
// allowWrite paths → rw, allowRead paths → ro, CWD → rw/ro per access field.
// projectGitDirs are added as rw so git commands work in worktrees whose .git files
// point back to the main repo's .git dir.
// Paths appearing in multiple lists are deduplicated (rw wins).
func BuildAgentSandboxPaths(
	sb *config.SandboxConfig, cwd, access string, projectGitDirs []string,
) []logos.AllowedPath {
	isCwdReadOnly := access != "rw"

	seen := make(map[string]bool)
	var ordered []string

	addPath := func(p string, readOnly bool) {
		if !filepath.IsAbs(p) {
			return
		}
		if existing, ok := seen[p]; ok {
			if existing && !readOnly {
				seen[p] = false
			}
			return
		}
		seen[p] = readOnly
		ordered = append(ordered, p)
	}

	addPath(cwd, isCwdReadOnly)

	for _, p := range sb.ExpandedAllowWrite() {
		addPath(p, false)
	}
	for _, p := range sb.ExpandedAllowRead() {
		addPath(p, true)
	}

	for _, gitDir := range projectGitDirs {
		addPath(gitDir, false)
	}

	paths := make([]logos.AllowedPath, 0, len(ordered))
	for _, p := range ordered {
		paths = append(paths, logos.AllowedPath{Path: p, ReadOnly: seen[p]})
	}
	return paths
}

// CollectProjectGitDirs returns deduplicated .git directories for all registered projects.
func CollectProjectGitDirs() []string {
	return project.ListGitDirs()
}
