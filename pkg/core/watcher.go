package core

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitora diretórios e promove arquivos a DISCOVERED quando o tamanho
// permanece estável por um intervalo configurável (RF01, seção 4.3).
type Watcher struct {
	fsw       *fsnotify.Watcher
	stableFor time.Duration
	onReady   func(path string)
	logger    *slog.Logger
	dirs      []string // raízes monitoradas, usadas na varredura periódica
	scanEvery time.Duration
	scanned   map[string]bool // arquivos já promovidos nesta execução
	done      chan struct{}
	mu        sync.Mutex
	pending   map[string]*stableTracker
}

type stableTracker struct {
	lastSize    int64
	lastUpdated time.Time
}

// NewWatcher cria um watcher com o debounce configurado.
// scanEvery é o intervalo da varredura periódica de fallback; 0 desliga.
func NewWatcher(stableFor time.Duration, onReady func(path string), logger *slog.Logger, scanEvery time.Duration) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		fsw:       fsw,
		stableFor: stableFor,
		onReady:   onReady,
		logger:    logger,
		scanEvery: scanEvery,
		scanned:   make(map[string]bool),
		done:      make(chan struct{}),
		pending:   make(map[string]*stableTracker),
	}, nil
}

// AddDir passa a monitorar um diretório (recursivamente) em busca de arquivos de mídia.
func (w *Watcher) AddDir(dir string) error {
	w.mu.Lock()
	w.dirs = append(w.dirs, dir)
	w.mu.Unlock()
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			w.logger.Warn("falha ao acessar caminho durante varredura", "path", path, "error", err.Error())
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if err := w.fsw.Add(path); err != nil {
			w.logger.Error("falha ao adicionar diretório ao watcher", "dir", path, "error", err.Error())
			return err
		}
		w.logger.Info("monitorando diretório", "dir", path)
		return nil
	})
}

// scan varre as raízes monitoradas e roteia arquivos suportados pelo mesmo
// mecanismo de estabilidade, servindo de fallback para eventos que o fsnotify
// não entregou (ex.: arquivos já presentes no start ou renames externos).
func (w *Watcher) scan() {
	var roots []string
	w.mu.Lock()
	roots = append(roots, w.dirs...)
	w.mu.Unlock()
	if len(roots) == 0 {
		return
	}
	var files []string
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				_ = w.fsw.Add(path) // pega subpastas criadas depois do start
				return nil
			}
			if isSupported(path) {
				files = append(files, path)
			}
			return nil
		})
	}
	for _, f := range files {
		w.mu.Lock()
		done := w.scanned[f]
		w.mu.Unlock()
		if done {
			continue
		}
		w.touch(f)
	}
}

// Start dispara o loop de eventos do watcher em background.
func (w *Watcher) Start() {
	go w.run()
}

// Close encerra o watcher e seus loops.
func (w *Watcher) Close() {
	close(w.done)
	_ = w.fsw.Close()
}

func (w *Watcher) run() {
	debounce := time.NewTicker(500 * time.Millisecond)
	defer debounce.Stop()

	// varredura imediata no start pega arquivos já presentes que o fsnotify
	// nunca notificaria
	w.scan()

	var poll *time.Ticker
	var pollC <-chan time.Time
	if w.scanEvery > 0 {
		poll = time.NewTicker(w.scanEvery)
		pollC = poll.C
		defer poll.Stop()
	}

	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.logger.Debug("evento do watcher", "path", ev.Name, "op", ev.Op.String())
			switch {
			case ev.Op&(fsnotify.Write|fsnotify.Create) != 0 && isSupported(ev.Name):
				w.touch(ev.Name)
			case ev.Op&fsnotify.Rename != 0:
				// O arquivo foi movido/renomeado (ex.: .mkv.part -> .mkv).
				// Descartamos o rastreio da origem; o CREATE do destino cuida dele.
				w.Remove(ev.Name)
			}
		case err, ok := <-w.fsw.Errors:
			if ok && err != nil && err != os.ErrClosed {
				w.logger.Error("erro do filesystem watcher", "error", err.Error())
			}
		case <-debounce.C:
			w.flushStable()
		case <-pollC:
			w.scan()
		}
	}
}

func (w *Watcher) touch(path string) {
	size, err := fileSize(path)
	if err != nil {
		w.logger.Debug("fileSize falhou ao tocar", "path", path, "error", err.Error())
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logger.Debug("arquivo tocado", "path", path, "size", size)
	cur, ok := w.pending[path]
	if !ok {
		w.pending[path] = &stableTracker{lastSize: size, lastUpdated: time.Now()}
		return
	}
	if cur.lastSize == size {
		if time.Since(cur.lastUpdated) >= w.stableFor {
			w.promote(path)
		}
	} else {
		cur.lastSize = size
		cur.lastUpdated = time.Now()
	}
}

func (w *Watcher) flushStable() {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	for path, t := range w.pending {
		if _, err := os.Stat(path); err != nil {
			// arquivo sumiu/renomeado desde o último evento; descarta rastreio
			w.logger.Debug("arquivo não existe mais; descartando rastreio", "path", path, "error", err.Error())
			delete(w.pending, path)
			continue
		}
		if now.Sub(t.lastUpdated) >= w.stableFor {
			w.promote(path)
		}
	}
}

func (w *Watcher) promote(path string) {
	w.logger.Info("arquivo estável - promovendo à descoberta", "path", path, "stable_for", w.stableFor.String())
	if w.onReady != nil {
		w.onReady(path)
	}
	w.scanned[path] = true
	delete(w.pending, path)
}

// Reset forgotten resets a pending tracker (usado em testes/limpeza).
func (w *Watcher) Remove(path string) {
	w.mu.Lock()
	delete(w.pending, path)
	w.mu.Unlock()
}

// RemoveDir para de monitorar dir (e todos os seus descendentes) e purga
// qualquer rastreio pendente/já promovido sob esse caminho. Usado pelo
// painel de controle (Fase C — Diretórios Monitorados) quando o usuário
// remove um diretório da lista.
//
// Refaz filepath.Walk(dir) NO MOMENTO da remoção, em vez de reusar um
// snapshot de subpastas capturado quando AddDir foi chamado: scan()
// registra novas subpastas no fsnotify.Watcher conforme elas são criadas
// (w.fsw.Add em scan()), então a árvore real pode ter crescido desde o
// AddDir original — um snapshot antigo deixaria subpastas novas ainda
// registradas no fsnotify após a remoção.
//
// Se dir não existir mais em disco (ex.: apagado externamente), a
// varredura falha silenciosamente (só loga) mas w.dirs/w.pending/w.scanned
// ainda são purgados — a remoção lógica não depende do diretório existir
// fisicamente.
func (w *Watcher) RemoveDir(dir string) error {
	dir = filepath.Clean(dir)

	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			w.logger.Warn("falha ao acessar caminho durante remoção", "path", path, "error", err.Error())
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if rmErr := w.fsw.Remove(path); rmErr != nil {
			// Comum e inofensivo: o path pode já ter sido removido do
			// fsnotify por outro evento (ex.: rename/delete externo).
			w.logger.Debug("falha ao remover diretório do watcher", "dir", path, "error", rmErr.Error())
		}
		return nil
	})

	w.mu.Lock()
	defer w.mu.Unlock()

	filtered := w.dirs[:0]
	for _, d := range w.dirs {
		if d != dir {
			filtered = append(filtered, d)
		}
	}
	w.dirs = filtered

	prefix := dir + string(filepath.Separator)
	for p := range w.pending {
		if p == dir || strings.HasPrefix(p, prefix) {
			delete(w.pending, p)
		}
	}
	for p := range w.scanned {
		if p == dir || strings.HasPrefix(p, prefix) {
			delete(w.scanned, p)
		}
	}

	w.logger.Info("diretório removido do monitoramento", "dir", dir)
	return walkErr
}

var supportedExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".webm": true, ".ts": true, ".m2ts": true, ".m4v": true,
}

// partialSuffixes identifica extensões usadas por downloads/cópias em andamento
// (ex.: "arquivo.mkv.part"). Irrelevantes para isSupported quando o base é mídia.
var partialSuffixes = []string{
	".part", ".crdownload", ".download", ".partial", ".filepart",
	".tmp", ".temp",
}

// isSupported aceita arquivos cuja extensão final é de mídia, além de arquivos
// parciais cujo nome-base é uma extensão de mídia (ex.: "x.mkv.part"). Isso
// garante que cópias/downloads com sufixo temporário sejam rastreados.
func isSupported(path string) bool {
	lower := strings.ToLower(path)
	ext := filepath.Ext(lower)
	if supportedExts[ext] {
		return true
	}
	// extensões finais de mídia com sufixo parcial/cópia: "x.mkv.part"
	for _, s := range partialSuffixes {
		if strings.HasSuffix(lower, s) {
			return supportedExts[filepath.Ext(strings.TrimSuffix(lower, s))]
		}
	}
	return false
}

// IsSupportedMedia expõe isSupported para consumidores fora do pacote (ex.:
// cmd/cli, no modo -health-check standalone), sem alterar a lógica interna
// já usada por scan()/run().
func IsSupportedMedia(path string) bool {
	return isSupported(path)
}

// DiscoverFiles varre recursivamente cada diretório de dirs e retorna todos
// os arquivos cuja extensão é suportada (mesmo filtro de isSupported usado
// pelo watcher em tempo real), em uma única passada "one-shot" — não depende
// de fsnotify, callbacks ou de uma instância de Watcher. Usado pelo modo
// -health-check standalone (cmd/cli) para descobrir arquivos sem monitorar
// diretórios continuamente.
func DiscoverFiles(dirs []string) ([]string, error) {
	var files []string
	for _, root := range dirs {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if isSupported(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
