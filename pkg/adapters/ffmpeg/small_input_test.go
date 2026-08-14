//go:build integration

package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/FM0Ura/codecany/pkg/core"
)

// getSmallInput localiza o arquivo real small_input_test.mkv (gitignored) na
// raiz do repositório. Permite override via CODECANY_TEST_VIDEO. Quando
// indisponível (ex.: checkout de CI sem o arquivo), pula o teste.
func getSmallInput(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não disponível")
	}
	if p := os.Getenv("CODECANY_TEST_VIDEO"); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("CODECANY_TEST_VIDEO definido mas inexistente: %s", p)
		}
		return p
	}
	p := filepath.Join("..", "..", "..", "small_input_test.mkv")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("small_input_test.mkv não encontrado (gitignored); use CODECANY_TEST_VIDEO")
	}
	return p
}

func TestSmallInputProbe(t *testing.T) {
	path := getSmallInput(t)
	mi, err := NewProber().Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	if mi.Container != "matroska" && mi.Container != "matroska,webm" {
		t.Errorf("esperava container matroska, obteve %q", mi.Container)
	}
	if !mi.HasVideo || mi.VideoCodec != "h264" {
		t.Errorf("esperava vídeo h264, obteve video=%v codec=%q", mi.HasVideo, mi.VideoCodec)
	}
	if !mi.HasAudio {
		t.Errorf("esperava trilha de áudio")
	}
	if mi.Width != 1920 || mi.Height != 1080 {
		t.Errorf("resolução esperada 1920x1080, obteve %dx%d", mi.Width, mi.Height)
	}
	if mi.DurationSec < 100 {
		t.Errorf("duração esperada acima de 100s, obteve %v", mi.DurationSec)
	}
}

func TestSmallInputTranscode(t *testing.T) {
	path := getSmallInput(t)
	mi, err := NewProber().Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.mkv")
	tc := NewTranscode()
	prog, err := tc.Transcode(path, out, mi, core.TargetSpec{VideoCodec: "libx264", VideoCRF: 30, VideoPreset: "veryfast"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Minute)
	completed := false
	for {
		select {
		case p, ok := <-prog:
			if !ok {
				completed = true
			} else if p == 1.0 {
				completed = true
			}
		case <-deadline:
			t.Fatal("tempo esgotado no transcode do small_input_test.mkv")
		}
		if completed {
			break
		}
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output não produzido: %v", err)
	}
	if fi.Size() == 0 {
		t.Errorf("output vazio")
	}
}
