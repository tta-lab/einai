package daemon

import (
	"net/http"
	"os"
	"strconv"

	"github.com/tta-lab/einai/internal/jobqueue"
)

func (d *Daemon) handleJobList(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	jobs := d.queue.List(limit)
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (d *Daemon) handleJobLog(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	job, ok := d.queue.Get(id)
	if !ok {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(job.OutputPath)
	if err != nil {
		http.Error(w, `{"error":"output file not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data) //nolint:errcheck
}

func (d *Daemon) handleJobKill(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing id"})
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid id"})
		return
	}

	err = d.worker.Kill(id)
	if err != nil {
		if err == jobqueue.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "job not found"})
			return
		}
		if err == jobqueue.ErrNotRunning {
			writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "job not in running state"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
