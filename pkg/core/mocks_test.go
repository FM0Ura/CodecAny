package core

import (
	"os"
	"time"
)

// mockProber implementa MediaProber com resultado determinístico.
type mockProber struct {
	info MediaInfo
	err  error
}

func (m mockProber) Probe(path string) (MediaInfo, error) { return m.info, m.err }

// mockTranscoder implementa TranscoderEngine, gravando um output com o
// tamanho configurado e fechando o canal de progresso após emitir 1.0.
type mockTranscoder struct {
	outputSize int64
}

func (m *mockTranscoder) Name() string { return "mock" }

func (m *mockTranscoder) Transcode(input, output string, media MediaInfo, target TargetSpec) (<-chan float64, error) {
	f, err := os.Create(output)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, m.outputSize)
	if _, err := f.Write(buf); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	prog := make(chan float64, 1)
	prog <- 1.0
	close(prog)
	return prog, nil
}

// mockVerifier implementa MediaVerifier com resposta determinística.
type mockVerifier struct {
	err error
}

func (m mockVerifier) Verify(path string) error { return m.err }

// drainEvents consome o canal de eventos até um limite de tempo.
func drainEvents(ch chan JobEvent, timeout time.Duration) []JobEvent {
	var out []JobEvent
	tl := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-tl:
			return out
		}
	}
}
