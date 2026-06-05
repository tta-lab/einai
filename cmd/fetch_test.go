package cmd

import (
	"reflect"
	"testing"
)

func TestBuildFetchArgs(t *testing.T) {
	got := buildFetchArgs([]string{"https://example.com", "--tree"})
	want := []string{"fetch", "https://example.com", "--tree"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildFetchArgs() = %v, want %v", got, want)
	}
}
