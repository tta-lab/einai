package cmd

import (
	"reflect"
	"testing"
)

func TestBuildFetchArgs(t *testing.T) {
	got := buildFetchArgs("https://example.com", "deepseek/deepseek-v4-flash")
	want := []string{
		"run",
		"--agent",
		"webdiver",
		"--readonly",
		"--small-model",
		"-m",
		"deepseek/deepseek-v4-flash",
		"--",
		"Fetch and analyze https://example.com",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildFetchArgs() = %v, want %v", got, want)
	}
}
