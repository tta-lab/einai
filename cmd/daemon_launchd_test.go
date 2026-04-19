package cmd

import (
	"fmt"
	"strings"
	"testing"
)

// assertPathContainsAll checks that all dirs are present in the PATH string.
func assertPathContainsAll(t *testing.T, path string, dirs []string) {
	t.Helper()
	for _, dir := range dirs {
		if !strings.Contains(path, dir) {
			t.Errorf("PATH missing %q: %s", dir, path)
		}
	}
}

// assertPathCountOnce checks that dir appears exactly once in PATH.
func assertPathCountOnce(t *testing.T, path, dir string) {
	t.Helper()
	count := strings.Count(":"+path+":", ":"+dir+":")
	if count != 1 {
		t.Errorf("expected %q once in PATH, got %d times: %s", dir, count, path)
	}
}

// assertExtraDirsAfterBase checks that truly-extra dirs (not already in base) appear after base dirs.
func assertExtraDirsAfterBase(t *testing.T, path string, base, extra []string) {
	t.Helper()
	if len(base) == 0 || len(extra) == 0 {
		return
	}
	baseSet := make(map[string]bool, len(base))
	for _, b := range base {
		baseSet[b] = true
	}
	lastBaseIdx := strings.LastIndex(path, base[len(base)-1])
	for _, e := range extra {
		if baseSet[e] {
			continue
		}
		if eIdx := strings.Index(path, e); eIdx >= 0 && eIdx < lastBaseIdx {
			t.Errorf("extra dir %q appears before base dirs in PATH: %s", e, path)
		}
	}
}

func TestDefaultPATHDirs_includesLocalBin(t *testing.T) {
	// init() should have prepended $HOME/.local/bin
	if len(defaultPATHDirs) == 0 || !strings.HasSuffix(defaultPATHDirs[0], "/.local/bin") {
		t.Errorf("expected defaultPATHDirs[0] to be $HOME/.local/bin, got %v", defaultPATHDirs)
	}
}

func TestBuildPATH(t *testing.T) {
	tests := []struct {
		name     string
		base     []string
		extra    []string
		wantPath string
		wantIn   []string
		wantOnce string
	}{
		{
			name:     "empty extra returns base only",
			base:     []string{"/usr/bin", "/bin"},
			extra:    nil,
			wantPath: "/usr/bin:/bin",
		},
		{
			name:   "extra dirs appended after base",
			base:   []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/bin"},
			extra:  []string{"/Users/neil/.local/bin"},
			wantIn: []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/bin", "/Users/neil/.local/bin"},
		},
		{
			name:     "duplicate in extra is deduplicated",
			base:     []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/bin"},
			extra:    []string{"/opt/homebrew/bin", "/Users/neil/.local/bin"},
			wantOnce: "/opt/homebrew/bin",
			wantIn:   []string{"/usr/local/bin", "/Users/neil/.local/bin"},
		},
		{
			name:   "extra appears after base dirs",
			base:   []string{"/usr/local/bin"},
			extra:  []string{"/Users/neil/.local/bin"},
			wantIn: []string{"/usr/local/bin", "/Users/neil/.local/bin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPATH(tt.base, tt.extra)
			if tt.wantPath != "" && got != tt.wantPath {
				t.Errorf("buildPATH() = %q, want %q", got, tt.wantPath)
			}
			assertPathContainsAll(t, got, tt.wantIn)
			if tt.wantOnce != "" {
				assertPathCountOnce(t, got, tt.wantOnce)
			}
			assertExtraDirsAfterBase(t, got, tt.base, tt.extra)
		})
	}
}

func TestDiscoverBinaryDirs(t *testing.T) {
	tests := []struct {
		name      string
		names     []string
		lookupFn  func(string) (string, error)
		wantDirs  []string
		wantEmpty bool
	}{
		{
			name:  "single binary found",
			names: []string{"claude"},
			lookupFn: func(name string) (string, error) {
				return "/Users/neil/.local/bin/claude", nil
			},
			wantDirs: []string{"/Users/neil/.local/bin"},
		},
		{
			name:  "binary not found skipped gracefully",
			names: []string{"missing-binary"},
			lookupFn: func(name string) (string, error) {
				return "", fmt.Errorf("not found")
			},
			wantEmpty: true,
		},
		{
			name:  "two binaries in same dir deduped",
			names: []string{"claude", "codex"},
			lookupFn: func(name string) (string, error) {
				return "/opt/homebrew/bin/" + name, nil
			},
			wantDirs: []string{"/opt/homebrew/bin"},
		},
		{
			name:  "two binaries in different dirs both returned",
			names: []string{"claude", "codex"},
			lookupFn: func(name string) (string, error) {
				paths := map[string]string{
					"claude": "/Users/neil/.local/bin/claude",
					"codex":  "/opt/homebrew/bin/codex",
				}
				p, ok := paths[name]
				if !ok {
					return "", fmt.Errorf("not found")
				}
				return p, nil
			},
			wantDirs: []string{"/Users/neil/.local/bin", "/opt/homebrew/bin"},
		},
		{
			name:  "empty name list returns empty",
			names: []string{},
			lookupFn: func(name string) (string, error) {
				return "/usr/bin/" + name, nil
			},
			wantEmpty: true,
		},
		{
			name:  "binary path with no slash skipped",
			names: []string{"weird"},
			lookupFn: func(name string) (string, error) {
				return "nopath", nil
			},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := whichLookup
			whichLookup = tt.lookupFn
			defer func() { whichLookup = orig }()

			got := discoverBinaryDirs(tt.names)

			if tt.wantEmpty {
				if len(got) != 0 {
					t.Errorf("expected empty dirs, got %v", got)
				}
				return
			}
			for _, want := range tt.wantDirs {
				found := false
				for _, g := range got {
					if g == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected dir %q in result %v", want, got)
				}
			}
			if len(got) != len(tt.wantDirs) {
				t.Errorf("expected %d dirs, got %d: %v", len(tt.wantDirs), len(got), got)
			}
		})
	}
}

func TestGeneratePlist(t *testing.T) {
	tests := []struct {
		name       string
		binaryPath string
		extraDirs  []string
		wantIn     []string
		eiDirFirst bool
	}{
		{
			name:       "contains binary path and PATH key",
			binaryPath: "/usr/local/bin/ei",
			extraDirs:  nil,
			wantIn:     []string{"/usr/local/bin/ei", "PATH", ".local/bin"},
		},
		{
			name:       "ei binary dir prepended before base dirs",
			binaryPath: "/Users/neil/.local/bin/ei",
			extraDirs:  nil,
			wantIn:     []string{"/Users/neil/.local/bin"},
			eiDirFirst: true,
		},
		{
			name:       "extra dirs included in PATH",
			binaryPath: "/usr/local/bin/ei",
			extraDirs:  []string{"/custom/dir"},
			wantIn:     []string{"/custom/dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plist := generatePlist(tt.binaryPath, tt.extraDirs)
			assertPathContainsAll(t, plist, tt.wantIn)
			if tt.eiDirFirst {
				eiDir := tt.binaryPath[:strings.LastIndex(tt.binaryPath, "/")]
				eiIdx := strings.Index(plist, eiDir)
				baseIdx := strings.Index(plist, defaultPATHDirs[0])
				if eiIdx < 0 {
					t.Errorf("eiDir %q not found in plist", eiDir)
				} else if eiIdx > baseIdx {
					t.Errorf("eiDir %q appears after first base dir %q; expected prepend", eiDir, defaultPATHDirs[0])
				}
			}
		})
	}
}
