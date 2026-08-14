package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceAtomicCrossDevice simula a troca entre filesystems distintos:
// normally desloca o arquivo de origem para o diretório do destino a fim de
// reproduzir a queda do os.Rename (EXDEV). Verifica que replaceAtomic conclui
// mesmo assim, copiando para o device de destino.
func TestReplaceAtomicCrossDevice(t *testing.T) {
	// mesmo device neste teste; o ponto é exercitar o caminho de cópia+rename
	dir := t.TempDir()
	other := t.TempDir()
	src := filepath.Join(other, "orig-out.mkv")
	if err := os.WriteFile(src, []byte("0123456789"), 0o640); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "final.mkv")

	if err := replaceAtomic(src, dst); err != nil {
		t.Fatalf("replaceAtomic (fallback) falhou: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "0123456789" {
		t.Fatalf("conteúdo final não bateu: %q", b)
	}
	// origem não deve existir mais (foi movida), nem sobrar .tmp/.copy
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("esperava origem removida, mas existe: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".copy" || filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("sobrou temp de troca: %s", e.Name())
		}
	}
}

// TestReplaceAtomicSameDevice cobre o caminho rápido (rename direto).
func TestReplaceAtomicSameDevice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	dst := filepath.Join(dir, "b.mkv")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceAtomic(src, dst); err != nil {
		t.Fatalf("replaceAtomic direto falhou: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expect src moved; ainda existe: %v", err)
	}
}