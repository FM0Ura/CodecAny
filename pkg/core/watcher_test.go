package core

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestIsSupported(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"movie.mkv", true},
		{"sub/other.MP4", true},
		{"noext", false},
		{"movie.txt", false},
		{"pew.twitter", false},
		{"movie.mkv.part", true},      // cópia parcial em andamento
		{"movie.mkv.crdownload", true}, // download em andamento
		{"movie.mkv.tmp", true},       // temp
		{"notes.part", false},         // parcial sem base de mídia
		{"image.mkv.jpg", false},      // base não-renomeável (ext final não media)
	}
	for _, c := range cases {
		if got := isSupported(c.path); got != c.want {
			t.Errorf("isSupported(%q) = %v, esperava %v", c.path, got, c.want)
		}
	}
}
func newTestWatcher(t *testing.T, dir string, onReady func(path string)) *Watcher {
	t.Helper()
	w, err := NewWatcher(500*time.Millisecond, onReady, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	return w
}

func TestScanDetectsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFileSize(t, a, 1000)
	b := filepath.Join(dir, "n.tmp")
	writeFileSize(t, b, 2000) // temp, não media

	var got []string
	w := newTestWatcher(t, dir, func(p string) { got = append(got, p) })
	if err := w.AddDir(dir); err != nil {
		t.Fatal(err)
	}
	w.scan() // síncrono, sem estabilidade ainda (1000 no pending)

	// força a estabilização: toca de novo depois de stableFor
	time.Sleep(600 * time.Millisecond)
	w.touch(a)
	w.touch(b)

	if len(got) != 1 || got[0] != a {
		t.Fatalf("esperava somente %s promovido, obteve %v", a, got)
	}
}

func TestScanSkipsAlreadyPromoted(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFileSize(t, a, 1000)

	var count int
	w := newTestWatcher(t, dir, func(p string) { count++ })
	if err := w.AddDir(dir); err != nil {
		t.Fatal(err)
	}
	w.touch(a)
	time.Sleep(600 * time.Millisecond)
	w.touch(a) // promove e marca scanned
	if count != 1 {
		t.Fatalf("promoveu %d vezes, esperava 1", count)
	}
	w.scan() // não deve re-promover nem re-tocar arquivo scanned
	if count != 1 {
		t.Fatalf("scan reprocessou arquivo scanned: count=%d", count)
	}
}

// TestDiscoverFilesFiltersAndRecurses exercita a versão "one-shot" de varredura
// usada pelo modo -health-check standalone (Fase 5): mesma lista de extensões
// suportadas e mesma lógica de sufixo parcial/temporário de isSupported, sem
// depender de fsnotify/callbacks/instância de Watcher, e recursiva em
// subdiretórios.
func TestDiscoverFilesFiltersAndRecurses(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "a.mkv")
	writeFileSize(t, ok, 1000)
	notMedia := filepath.Join(dir, "readme.txt")
	writeFileSize(t, notMedia, 10)
	// "b.mkv.part" tem base de mídia (b.mkv) com sufixo parcial/temporário:
	// isSupported trata como suportado (cópia em andamento de um arquivo de
	// mídia) — DiscoverFiles precisa espelhar exatamente essa lógica.
	partialWithMediaBase := filepath.Join(dir, "b.mkv.part")
	writeFileSize(t, partialWithMediaBase, 500)
	// "notes.part" não tem base de mídia: deve ser ignorado.
	partialWithoutMediaBase := filepath.Join(dir, "notes.part")
	writeFileSize(t, partialWithoutMediaBase, 500)

	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(sub, "c.mp4")
	writeFileSize(t, nested, 2000)

	got, err := DiscoverFiles([]string{dir})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{ok: true, nested: true, partialWithMediaBase: true}
	if len(got) != len(want) {
		t.Fatalf("esperava %d arquivos, obteve %d: %v", len(want), len(got), got)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("DiscoverFiles retornou arquivo inesperado: %s", f)
		}
	}
}

// TestDiscoverFilesMultipleDirs confirma que múltiplos diretórios são
// varridos e seus resultados combinados.
func TestDiscoverFilesMultipleDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	a := filepath.Join(dirA, "a.mkv")
	writeFileSize(t, a, 1000)
	b := filepath.Join(dirB, "b.avi")
	writeFileSize(t, b, 1000)

	got, err := DiscoverFiles([]string{dirA, dirB})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 arquivos, obteve %d: %v", len(got), got)
	}
}

// TestIsSupportedMediaWrapper garante que o wrapper exportado espelha
// isSupported sem alterar o comportamento.
func TestIsSupportedMediaWrapper(t *testing.T) {
	if !IsSupportedMedia("movie.mkv") {
		t.Error("IsSupportedMedia(movie.mkv) deveria ser true")
	}
	if IsSupportedMedia("notes.txt") {
		t.Error("IsSupportedMedia(notes.txt) deveria ser false")
	}
}

func TestScanCoversNewSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "sub", "b.mkv")
	writeFileSize(t, f, 1000)

	var got []string
	w := newTestWatcher(t, dir, func(p string) { got = append(got, p) })
	if err := w.AddDir(dir); err != nil {
		t.Fatal(err)
	}
	w.scan()
	time.Sleep(600 * time.Millisecond)
	w.flushStable()

	if len(got) != 1 || got[0] != f {
		t.Fatalf("esperava %s promovido, obteve %v", f, got)
	}
}

// TestWatcherRemoveDir cobre o item 5 da proposta do painel de controle
// (Fase C): AddDir → RemoveDir → um arquivo criado DEPOIS da remoção não
// deve mais ser promovido/descoberto, e o estado interno (w.dirs/w.pending/
// w.scanned) precisa estar limpo sob o caminho removido.
func TestWatcherRemoveDir(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	writeFileSize(t, a, 1000)

	var mu sync.Mutex
	var got []string
	w := newTestWatcher(t, dir, func(p string) {
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	})
	if err := w.AddDir(dir); err != nil {
		t.Fatal(err)
	}
	w.scan()
	time.Sleep(600 * time.Millisecond)
	w.flushStable()

	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 1 || got[0] != a {
		t.Fatalf("esperava %s promovido antes da remoção, obteve %v", a, got)
	}

	if err := w.RemoveDir(dir); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}

	w.mu.Lock()
	if len(w.dirs) != 0 {
		t.Errorf("w.dirs deveria estar vazio após RemoveDir, obteve %v", w.dirs)
	}
	if len(w.scanned) != 0 {
		t.Errorf("w.scanned deveria estar vazio após RemoveDir, obteve %v", w.scanned)
	}
	if len(w.pending) != 0 {
		t.Errorf("w.pending deveria estar vazio após RemoveDir, obteve %v", w.pending)
	}
	w.mu.Unlock()

	// Arquivo criado DEPOIS da remoção não deve ser promovido: scan() não
	// varre mais este diretório (fora de w.dirs) e ele não foi
	// re-adicionado ao fsnotify.
	b := filepath.Join(dir, "b.mkv")
	writeFileSize(t, b, 2000)
	w.scan()
	time.Sleep(600 * time.Millisecond)
	w.flushStable()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("arquivo criado após RemoveDir foi promovido indevidamente: %v", got)
	}
}

// TestWatcherRemoveDirRefreshesSubdirs garante que RemoveDir refaz a
// varredura no momento da remoção (não reusa um snapshot de AddDir): uma
// subpasta criada DEPOIS do AddDir original (e descoberta pela varredura
// periódica scan(), que chama w.fsw.Add em subpastas novas) também precisa
// ser purgada de w.pending/w.scanned na remoção.
func TestWatcherRemoveDirRefreshesSubdirs(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var got []string
	w := newTestWatcher(t, dir, func(p string) {
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	})
	if err := w.AddDir(dir); err != nil {
		t.Fatal(err)
	}

	// Subpasta criada DEPOIS do AddDir original — só entra em w.fsw via
	// scan() (mesmo mecanismo usado por TestScanCoversNewSubdir).
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(sub, "nested.mkv")
	writeFileSize(t, nested, 1500)
	w.scan() // registra "sub" no fsnotify e toca nested.mkv (pending)

	w.mu.Lock()
	_, pending := w.pending[nested]
	w.mu.Unlock()
	if !pending {
		t.Fatalf("esperava %s em w.pending após scan()", nested)
	}

	if err := w.RemoveDir(dir); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}

	w.mu.Lock()
	_, stillPending := w.pending[nested]
	w.mu.Unlock()
	if stillPending {
		t.Errorf("w.pending ainda contém %s (subpasta da varredura) após RemoveDir", nested)
	}

	// Deixa o arquivo estabilizar e confirma que nunca foi promovido.
	time.Sleep(600 * time.Millisecond)
	w.flushStable()
	w.scan()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 0 {
		t.Fatalf("nested.mkv não deveria ter sido promovido após RemoveDir: %v", got)
	}
}
