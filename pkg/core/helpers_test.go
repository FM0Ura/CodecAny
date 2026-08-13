package core

import (
	"os"
	"testing"
)

// writeFileSize cria um arquivo com exatamente size bytes e retorna o caminho.
func writeFileSize(t *testing.T, path string, size int64) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, size)
	if _, err := f.Write(buf); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	return path
}
