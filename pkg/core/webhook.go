package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookClient é responsável por enviar notificações de webhook com retentativas (seção 4.1).
type WebhookClient struct {
	url    string
	events map[string]bool
	client *http.Client
}

// NewWebhookClient inicializa o cliente com timeout de 5 segundos.
func NewWebhookClient(cfg WebhookConfig) *WebhookClient {
	evs := make(map[string]bool)
	for _, e := range cfg.Events {
		evs[e] = true
	}
	return &WebhookClient{
		url:    cfg.URL,
		events: evs,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// ShouldSend verifica se o webhook deve notificar sobre este evento.
func (w *WebhookClient) ShouldSend(eventKind string) bool {
	if w.url == "" {
		return false
	}
	if len(w.events) == 0 {
		return true // Se eventos estiver vazio, envia todos
	}
	return w.events[eventKind]
}

// Send tenta enviar o JSON do Job via POST. Em caso de falha, tenta até 3 vezes
// com intervalo de 2 segundos.
func (w *WebhookClient) Send(job *Job, eventKind string) error {
	if !w.ShouldSend(eventKind) {
		return nil
	}

	payload := struct {
		Event string `json:"event"`
		Job   *Job   `json:"job"`
	}{
		Event: eventKind,
		Job:   job,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest("POST", w.url, bytes.NewBuffer(data))
		if err != nil {
			return fmt.Errorf("criar request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.client.Do(req)
		if err == nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				resp.Body.Close()
				return nil
			}
			resp.Body.Close()
			lastErr = fmt.Errorf("status HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		if attempt < 3 {
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("envio de webhook falhou após 3 tentativas: %w", lastErr)
}
