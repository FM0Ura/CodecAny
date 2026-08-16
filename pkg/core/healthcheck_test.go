package core

import (
	"errors"
	"os"
	"testing"
)

// pathVerifier implementa MediaVerifier com respostas indexadas por path
// (diferente de mockVerifier em mocks_test.go, que só devolve um único err
// fixo para qualquer path) — necessário aqui para exercitar uma mistura de
// arquivos OK e corrompidos no mesmo TestRunHealthCheck.
type pathVerifier struct {
	errByPath map[string]error
}

func (p pathVerifier) Verify(path string) error { return p.errByPath[path] }

// TestRunHealthCheck cobre o caso central de RunHealthCheck (extraído de
// cmd/cli/main.go::runHealthCheck, Fase E): arquivo OK, arquivo corrompido e
// uma mistura de dirs+files na mesma chamada.
func TestRunHealthCheck(t *testing.T) {
	t.Run("arquivo OK", func(t *testing.T) {
		verifier := pathVerifier{}
		results, err := RunHealthCheck(nil, []string{"/media/a.mkv"}, verifier)
		if err != nil {
			t.Fatalf("RunHealthCheck retornou erro: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("esperava 1 resultado, obteve %d", len(results))
		}
		got := results[0]
		if got.Path != "/media/a.mkv" || got.Status != HealthStatusOK || got.Error != "" {
			t.Errorf("resultado inesperado: %+v", got)
		}
	})

	t.Run("arquivo corrompido", func(t *testing.T) {
		bad := "/media/bad.mkv"
		verifier := pathVerifier{errByPath: map[string]error{
			bad: errors.New("erro de decodificação"),
		}}
		results, err := RunHealthCheck(nil, []string{bad}, verifier)
		if err != nil {
			t.Fatalf("RunHealthCheck retornou erro: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("esperava 1 resultado, obteve %d", len(results))
		}
		got := results[0]
		if got.Path != bad || got.Status != HealthStatusCorrupted || got.Error != "erro de decodificação" {
			t.Errorf("resultado inesperado: %+v", got)
		}
	})

	t.Run("mistura de dirs e files", func(t *testing.T) {
		dir := t.TempDir()
		media := dir + "/movie.mkv"
		if err := os.WriteFile(media, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		notMedia := dir + "/readme.txt"
		if err := os.WriteFile(notMedia, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}

		extraOK := "/media/extra-ok.mp4"
		extraBad := "/media/extra-bad.mp4"
		verifier := pathVerifier{errByPath: map[string]error{
			extraBad: errors.New("boom"),
		}}

		results, err := RunHealthCheck([]string{dir}, []string{extraOK, extraBad}, verifier)
		if err != nil {
			t.Fatalf("RunHealthCheck retornou erro: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("esperava 3 resultados (1 descoberto via dir + 2 files), obteve %d: %+v", len(results), results)
		}

		byPath := make(map[string]HealthCheckResult, len(results))
		for _, r := range results {
			byPath[r.Path] = r
		}

		if _, ok := byPath[notMedia]; ok {
			t.Errorf("readme.txt não deveria ter sido descoberto: %+v", results)
		}
		if r, ok := byPath[media]; !ok || r.Status != HealthStatusOK {
			t.Errorf("esperava %s com status ok; obteve %+v (presente=%v)", media, r, ok)
		}
		if r, ok := byPath[extraOK]; !ok || r.Status != HealthStatusOK {
			t.Errorf("esperava %s com status ok; obteve %+v (presente=%v)", extraOK, r, ok)
		}
		if r, ok := byPath[extraBad]; !ok || r.Status != HealthStatusCorrupted || r.Error != "boom" {
			t.Errorf("esperava %s com status corrupted e erro boom; obteve %+v (presente=%v)", extraBad, r, ok)
		}
	})

	t.Run("diretório inexistente propaga erro", func(t *testing.T) {
		verifier := pathVerifier{}
		missing := t.TempDir() + "/nao-existe/subpasta"
		_, err := RunHealthCheck([]string{missing}, nil, verifier)
		if err == nil {
			t.Fatal("esperava erro ao varrer diretório inexistente")
		}
	})
}
