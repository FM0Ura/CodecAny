package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestHandleCancelJobQueued(t *testing.T) {
	ts := newTestServer(t)
	job := ts.seedJob(t, core.StatusQueued, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSONNoBody(t, srv.URL+"/api/jobs/"+job.ID+"/cancel")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}

	reloaded, err := ts.store.FindByID(job.ID)
	if err != nil || reloaded == nil {
		t.Fatalf("FindByID: %v", err)
	}
	if reloaded.Status != core.StatusFailed {
		t.Errorf("status = %v, want FAILED", reloaded.Status)
	}
}

func TestHandleQueuePauseResume(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	// Initial status
	res, err := http.Get(srv.URL + "/api/queue/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var stat struct {
		Paused bool `json:"paused"`
	}
	_ = json.NewDecoder(res.Body).Decode(&stat)
	if stat.Paused {
		t.Error("esperava fila não pausada inicialmente")
	}

	// Pause
	respP := postJSONNoBody(t, srv.URL+"/api/queue/pause")
	defer respP.Body.Close()
	if respP.StatusCode != http.StatusOK {
		t.Fatalf("pause status = %d", respP.StatusCode)
	}
	if !ts.app.Engine.IsQueuePaused() {
		t.Error("esperava engine pausado")
	}

	// Resume
	respR := postJSONNoBody(t, srv.URL+"/api/queue/resume")
	defer respR.Body.Close()
	if respR.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d", respR.StatusCode)
	}
	if ts.app.Engine.IsQueuePaused() {
		t.Error("esperava engine despausado")
	}
}

func TestHandleCancelAllJobs(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	job1 := &core.Job{ID: "j1", Path: "/media/1.mkv", Status: core.StatusQueued, Driver: "ffmpeg", CreatedAt: time.Now()}
	job2 := &core.Job{ID: "j2", Path: "/media/2.mkv", Status: core.StatusQueued, Driver: "ffmpeg", CreatedAt: time.Now()}
	_ = ts.store.CreateJob(job1)
	_ = ts.store.CreateJob(job2)

	resp := postJSONNoBody(t, srv.URL+"/api/queue/cancel-all")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var res struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if !res.OK || res.Count != 2 {
		t.Errorf("res = %+v, want OK:true, Count:2", res)
	}
}


