package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/FM0Ura/codecany/pkg/core"
)

// clientBufSize é o tamanho do buffer de cada canal-cliente SSE registrado
// no Broadcaster. Pequeno de propósito: um cliente lento (ou uma conexão já
// morta cujo desregistro ainda não foi processado) simplesmente perde
// eventos — publish nunca bloqueia, ver publish.
const clientBufSize = 16

// Broadcaster drena o canal ÚNICO de eventos nativos do Engine
// (Engine.Events(), um único consumidor no processo inteiro) numa goroutine
// dedicada (ver Run) e replica cada evento para N clientes SSE registrados
// dinamicamente, sem alterar em nada o contrato do Engine.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[chan core.JobEvent]struct{}
}

// NewBroadcaster constrói um Broadcaster vazio, pronto para Run/ServeHTTP.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: make(map[chan core.JobEvent]struct{})}
}

// Run consome events até o canal fechar e publica cada evento recebido para
// todos os clientes registrados no momento. Deve rodar em goroutine própria
// — bloqueia até o canal fechar (na prática, até o processo encerrar, já que
// Engine.Shutdown não fecha o canal de eventos hoje).
func (b *Broadcaster) Run(events <-chan core.JobEvent) {
	for ev := range events {
		b.publish(ev)
	}
}

// publish faz o fan-out não bloqueante: cada cliente tem seu próprio
// select/default, então um consumidor lento nunca aplica back-pressure no
// dreno de eng.Events() (e, por consequência, no worker pool do Engine, que
// bloqueia em e.emit ao publicar nesse mesmo canal).
func (b *Broadcaster) publish(ev core.JobEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- ev:
		default:
			// cliente lento demais para acompanhar: descarta este evento para
			// este cliente específico, sem travar os demais nem o dreno.
		}
	}
}

// register cria e registra um novo canal-cliente bufferizado.
func (b *Broadcaster) register() chan core.JobEvent {
	ch := make(chan core.JobEvent, clientBufSize)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// unregister remove e fecha o canal-cliente (chamado ao final da conexão
// SSE, ver ServeHTTP).
func (b *Broadcaster) unregister(ch chan core.JobEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// ServeHTTP implementa o handler SSE de GET /api/events: registra um canal
// cliente, escreve frames "event: <Kind>\ndata: <JSON>\n\n" por evento
// recebido e desregistra o cliente ao r.Context().Done() (desconexão).
func (b *Broadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := b.register()
	defer b.unregister(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
