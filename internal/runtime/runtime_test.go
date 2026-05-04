package runtime_test

import (
	"testing"

	"github.com/tta-lab/einai/internal/runtime"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input   string
		want    runtime.Runtime
		wantErr bool
	}{
		{"lenos", runtime.Lenos, false},
		{"claude-code", runtime.ClaudeCode, false},
		{"", "", true},
		{"unknown", "", true},
		{"CC", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := runtime.Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Parse(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefault(t *testing.T) {
	if runtime.Default != runtime.Lenos {
		t.Errorf("Default runtime = %q, want %q", runtime.Default, runtime.Lenos)
	}
}
