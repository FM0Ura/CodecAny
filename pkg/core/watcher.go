package core

import (
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
	done      chan struct{}
	mu        sync.Mutex
	pending   map[string]*stableTracker
}

type stableTracker struct {
	lastSize    int64
	lastUpdated time.Time
}

// NewWatcher cria um watcher com o debounce configurado.
func NewWatcher(stableFor time.Duration, onReady func(path string)) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		fsw:       fsw,
		stableFor: stableFor,
		onReady:   onReady,
		done:      make(chan struct{}),
		pending:   make(map[string]*stableTracker),
	}, nil
}

// AddDir passa a monitorar um diretório (recursivamente) em busca de arquivos de mídia.
func (w *Watcher) AddDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		return w.fsw.Add(path)
	})
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
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 && isSupported(ev.Name) {
				w.touch(ev.Name)
			}
		case err, ok := <-w.fsw.Errors:
			if ok && err != nil && err != os.ErrClosed {
				// TODO: logging estruturado
			}
		case <-debounce.C:
			w.flushStable()
		}
	}
}

func (w *Watcher) touch(path string) {
	size, err := fileSize(path)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
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
		if now.Sub(t.lastUpdated) >= w.stableFor {
			w.promote(path)
		}
	}
}

func (w *Watcher) promote(path string) {
	if w.onReady != nil {
		w.onReady(path)
	}
	delete(w.pending, path)
}

// Reset forgotten resets a pending tracker (usado em testes/limpeza).
func (w *Watcher) Remove(path string) {
	w.mu.Lock()
	delete(w.pending, path)
	w.mu.Unlock()
}

var supportedExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".webm": true, ".ts": true, ".m2ts": true, ".m4v": true,
}

func isSupported(path string) bool {
	return supportedExts[strings.ToLower(filepath.Ext(path))]
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
