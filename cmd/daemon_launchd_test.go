package cmd

import (
	"strings"
	"testing"
)

func TestBuildPATH_DeduplicatesAndPreservesOrder(t *testing.T) {
	base := []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/bin"}
	extra := []string{"/opt/homebrew/bin", "/Users/neil/.local/bin"}
	got := buildPATH(base, extra)
	parts := strings.Split(got, ":")
	// /opt/homebrew/bin should appear only once
	count := 0
	for _, p := range parts {
		if p == "/opt/homebrew/bin" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected /opt/homebrew/bin to appear once, got %d times in PATH: %s", count, got)
	}
	// extra dir not in base should be appended
	if !strings.Contains(got, "/Users/neil/.local/bin") {
		t.Errorf("expected /Users/neil/.local/bin in PATH: %s", got)
	}
	// base dirs should come before extra
	localIdx := strings.Index(got, "/Users/neil/.local/bin")
	brewIdx := strings.Index(got, "/usr/local/bin")
	if localIdx < brewIdx {
		t.Errorf("extra dir appeared before base dir in PATH: %s", got)
	}
}

func TestBuildPATH_EmptyExtra(t *testing.T) {
	base := []string{"/usr/bin", "/bin"}
	got := buildPATH(base, nil)
	if got != "/usr/bin:/bin" {
		t.Errorf("unexpected PATH: %s", got)
	}
}

func TestGeneratePlist_ContainsBinaryPath(t *testing.T) {
	plist := generatePlist("/usr/local/bin/ei", nil)
	if !strings.Contains(plist, "/usr/local/bin/ei") {
		t.Errorf("plist missing binary path")
	}
	if !strings.Contains(plist, "PATH") {
		t.Errorf("plist missing PATH key")
	}
}

func TestGeneratePlist_EiDirInPath(t *testing.T) {
	plist := generatePlist("/Users/neil/.local/bin/ei", nil)
	if !strings.Contains(plist, "/Users/neil/.local/bin") {
		t.Errorf("plist PATH missing ei binary dir: %s", plist)
	}
}

func TestGeneratePlist_ExtraDirsInPath(t *testing.T) {
	extra := []string{"/Users/neil/.local/bin"}
	plist := generatePlist("/usr/local/bin/ei", extra)
	if !strings.Contains(plist, "/Users/neil/.local/bin") {
		t.Errorf("plist PATH missing extra dir")
	}
}
