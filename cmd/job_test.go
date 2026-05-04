package cmd

import (
	"encoding/json"
	"testing"

	"github.com/tta-lab/einai/internal/jobqueue"
)

func TestJobListResponseParsing(t *testing.T) {
	data := `{"jobs":[
		{"id":1,"state":"queued","agent":"coder","runtime":"lenos"},
		{"id":2,"state":"completed","agent":"athena","runtime":"claude-code","send_target":"human"}
	]}`

	var result struct {
		Jobs []jobqueue.Job `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(result.Jobs))
	}
	if result.Jobs[0].State != jobqueue.StateQueued {
		t.Errorf("expected queued, got %v", result.Jobs[0].State)
	}
	if result.Jobs[1].SendTarget != "human" {
		t.Errorf("expected human, got %s", result.Jobs[1].SendTarget)
	}
}

func TestJobKillResponseParsing(t *testing.T) {
	tests := []struct {
		body    string
		wantOk  bool
		wantErr string
	}{
		{`{"ok":true}`, true, ""},
		{`{"ok":false,"error":"not found"}`, false, "not found"},
		{`{"ok":false,"error":"not running"}`, false, "not running"},
	}
	for _, tt := range tests {
		var r struct {
			Ok    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(tt.body), &r); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if r.Ok != tt.wantOk {
			t.Errorf("body=%q: ok=%v, want %v", tt.body, r.Ok, tt.wantOk)
		}
		if r.Error != tt.wantErr {
			t.Errorf("body=%q: error=%q, want %q", tt.body, r.Error, tt.wantErr)
		}
	}
}
