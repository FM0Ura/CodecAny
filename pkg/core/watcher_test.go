package core

import (
	"os"
	"path/filepath"
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
