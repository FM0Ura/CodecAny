package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookSendAndRetries(t *testing.T) {
	var attempts int
	var payload struct {
		Event string `json:"event"`
		Job   *Job   `json:"job"`
	}

	// 1. Servidor de teste com comportamento variável
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != "POST" {
			t.Errorf("esperava POST, obteve %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("esperava Content-Type application/json, obteve %s", r.Header.Get("Content-Type"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(body, &payload)

		// Falha nas duas primeiras tentativas, sucesso na terceira
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Configuração do Webhook
	cfg := WebhookConfig{
		URL:    ts.URL,
		Events: []string{"completed"},
	}
	client := NewWebhookClient(cfg)

	// Testa evento ignorado (não deve enviar)
	job := &Job{ID: "job-1", Path: "file.mkv"}
	err := client.Send(job, "failed")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Errorf("não deveria disparar webhook para evento 'failed', disparou %d vezes", attempts)
	}

	// Mede o tempo para garantir que houve sleep entre retentativas
	start := time.Now()
	err = client.Send(job, "completed")
	if err != nil {
		t.Fatalf("esperava sucesso na 3ª tentativa, obteve: %v", err)
	}
	duration := time.Since(start)

	if attempts != 3 {
		t.Errorf("esperava 3 tentativas no total, obteve %d", attempts)
	}
	if duration < 3*time.Second {
		t.Errorf("esperava backoff/sleep entre tentativas, total durou %v", duration)
	}
	if payload.Event != "completed" || payload.Job.ID != "job-1" {
		t.Errorf("payload incorreto ou incompleto: %+v", payload)
	}
}

func TestWebhookAllAttemptsFail(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	cfg := WebhookConfig{
		URL: ts.URL,
	}
	client := NewWebhookClient(cfg)

	job := &Job{ID: "job-2"}
	err := client.Send(job, "completed")
	if err == nil {
		t.Fatalf("esperava erro ao falhar todas as tentativas, obteve nil")
	}

	if attempts != 3 {
		t.Errorf("esperava tentar exatamente 3 vezes antes de desistir, tentou %d", attempts)
	}
}
