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

// geraVideo cria um pequeno vídeo H.264 com lavfi (requer ffmpeg real).
func geraVideo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não disponível")
	}
	path := filepath.Join(t.TempDir(), "sample.mp4")
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=25",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gerar vídeo: %v\n%s", err, out)
	}
	return path
}

func TestProbeIntegration(t *testing.T) {
	path := geraVideo(t)
	mi, err := NewProber().Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	if !mi.HasVideo || mi.VideoCodec == "" {
		t.Fatalf("esperava metadados de vídeo, obteve %+v", mi)
	}
	if mi.DurationSec < 1.0 || mi.DurationSec > 3.0 {
		t.Errorf("duração fora do esperado (1-3s): %v", mi.DurationSec)
	}
}

func TestTranscodeIntegration(t *testing.T) {
	path := geraVideo(t)
	mi, _ := NewProber().Probe(path)
	out := filepath.Join(filepath.Dir(path), "out.mp4")
	tc := NewTranscode()
	prog, err := tc.Transcode(path, out, mi, core.TargetSpec{VideoCodec: "libx264", VideoCRF: 28})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(30 * time.Second)
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
			t.Fatal("tempo esgotado no transcode")
		}
		if completed {
			break
		}
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output não produzido: %v", err)
	}
}
