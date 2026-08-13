// Command cli é o wrapper fino de linha de comando do CodecAny (seção 2/10).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/FM0Ura/codecany/pkg/adapters/ffmpeg"
	"github.com/FM0Ura/codecany/pkg/core"
)

func main() {
	var dirs multiFlag
	store := flag.String("db", "codecany.db", "arquivo SQLite (.db)")
	rules := flag.String("rules", "rules.yaml", "arquivo de regras YAML/JSON")
	workers := flag.Int("workers", 1, "número de workers")
	jsonLog := flag.Bool("json", false, "log estruturado em JSON")
	flag.Var(&dirs, "dir", "diretório a monitorar (repita para vários)")
	flag.Parse()
	if len(dirs) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	events := make(chan core.JobEvent, 256)
	go consumeEvents(events, *jsonLog)

	eng, err := buildEngine(*store, *rules, *workers, events)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, d := range dirs {
		if err := eng.WatchDir(d); err != nil {
			fmt.Fprintf(os.Stderr, "monitorar %s: %v\n", d, err)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Fprintln(os.Stderr, "shutdown... abortando jobs")
		eng.CloseCancellation()
	}()

	eng.StartWatch()
	eng.OpenWorkers(workCtx(sig))
	eng.Shutdown()
}

// workCtx fornece um context.Context cancelado ao receber sinal de término.
func workCtx(sig chan os.Signal) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sig
		cancel()
	}()
	return ctx
}

func buildEngine(storePath, rulesPath string, workers int, events chan core.JobEvent) (*core.Engine, error) {
	s, err := core.NewStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	re, err := core.NewRulesEngine(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	return core.NewEngine(core.EngineDeps{
		Store:   s,
		Rules:   re,
		Prober:  ffmpeg.NewProber(),
		Engines: []core.TranscoderEngine{ffmpeg.NewTranscode()},
		Workers: workers,
		Events:  events,
		Log:     os.Stderr,
	})
}

func consumeEvents(events chan core.JobEvent, jsonLog bool) {
	for ev := range events {
		if jsonLog {
			b, _ := json.Marshal(ev)
			fmt.Fprintln(os.Stderr, string(b))
			continue
		}
		id := ev.JobID
		if len(id) > 8 {
			id = id[:8]
		}
		switch ev.Kind {
		case core.EventJobStart:
			fmt.Fprintf(os.Stderr, "[job:%s] start %s\n", id, ev.FilePath)
		case core.EventJobProgress:
			fmt.Fprintf(os.Stderr, "[job:%s] %.0f%%\n", id, ev.Progress*100)
		case core.EventJobComplete:
			if ev.Success {
				fmt.Fprintf(os.Stderr, "[job:%s] completo %s (diff %d bytes)\n", id, ev.FilePath, ev.SizeDiff)
			} else {
				fmt.Fprintf(os.Stderr, "[job:%s] revertido %s (economia insuficiente)\n", id, ev.FilePath)
			}
		case core.EventJobError:
			fmt.Fprintf(os.Stderr, "[job:%s] erro: %s\n", id, ev.Error)
		}
	}
}

// multiFlag acumula valores repetidos de -dir.
type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint(*m) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
