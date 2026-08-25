package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FM0Ura/codecany/pkg/core"
)

func TestHandleRequeueJobFromFailed(t *testing.T) {
	ts := newTestServer(t)
	job := ts.seedJob(t, core.StatusFailed, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSONNoBody(t, srv.URL+"/api/jobs/"+job.ID+"/requeue")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	var got okResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK {
		t.Error("ok = false, want true")
	}

	reloaded, err := ts.store.FindByID(job.ID)
	if err != nil || reloaded == nil {
		t.Fatalf("FindByID: %v", err)
	}
	if reloaded.Status != core.StatusQueued {
		t.Errorf("status = %v, want QUEUED", reloaded.Status)
	}
}

func TestHandleRequeueJobFromRolledBack(t *testing.T) {
	ts := newTestServer(t)
	job := ts.seedJob(t, core.StatusRolledBack, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSONNoBody(t, srv.URL+"/api/jobs/"+job.ID+"/requeue")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
}

func TestHandleRequeueJobNotFound(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSONNoBody(t, srv.URL+"/api/jobs/id-inexistente/requeue")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleRequeueJobWrongStatus(t *testing.T) {
	ts := newTestServer(t)
	job := ts.seedJob(t, core.StatusCompleted, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSONNoBody(t, srv.URL+"/api/jobs/"+job.ID+"/requeue")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (job COMPLETED não é reenfileirável)", resp.StatusCode)
	}
}
