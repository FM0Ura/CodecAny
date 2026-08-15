//go:build integration

package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Cria legenda irmã temporária
	subPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".srt"
	if err := os.WriteFile(subPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello"), 0o600); err != nil {
		t.Fatal(err)
	}

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

	// Verifica se a legenda foi detectada
	if len(mi.SubtitlePaths) != 1 || mi.SubtitlePaths[0] != subPath {
		t.Errorf("esperava legenda em %s, obteve %v", subPath, mi.SubtitlePaths)
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

func TestTranscodeWithSubtitlesIntegration(t *testing.T) {
	// Cria vídeo MKV com lavfi para suportar legenda crua
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não disponível")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.mkv")
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=25",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gerar vídeo mkv: %v\n%s", err, out)
	}

	// Cria legenda srt irmã
	subPath := filepath.Join(dir, "sample.srt")
	if err := os.WriteFile(subPath, []byte("1\n00:00:00,500 --> 00:00:01,000\nSub text"), 0o600); err != nil {
		t.Fatal(err)
	}

	mi, err := NewProber().Probe(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(mi.SubtitlePaths) != 1 {
		t.Fatalf("esperava 1 legenda detectada, obteve %d", len(mi.SubtitlePaths))
	}

	out := filepath.Join(dir, "out.mkv")
	tc := NewTranscode()
	prog, err := tc.Transcode(path, out, mi, core.TargetSpec{VideoCodec: "libx264", VideoCRF: 35})
	if err != nil {
		t.Fatal(err)
	}

	// Aguarda conclusão
	deadline := time.After(15 * time.Second)
	completed := false
	for {
		select {
		case p, ok := <-prog:
			if !ok || p == 1.0 {
				completed = true
			}
		case <-deadline:
			t.Fatal("tempo esgotado no transcode com legendas")
		}
		if completed {
			break
		}
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output com legenda não produzido: %v", err)
	}

	// Verifica se a legenda foi embutida no arquivo final rodando ffprobe nele
	miOut, err := NewProber().Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	// O ffprobe original reportaria streams de legendas no JSON, mas como nosso
	// probe padrão filtra apenas video/audio, podemos validar a presença chamando
	// o ffprobe bruto e buscando por "subtitle" na saída.
	probeCmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", out)
	probeOut, err := probeCmd.Output()
	if err != nil {
		t.Fatalf("ffprobe no output: %v", err)
	}
	if !strings.Contains(string(probeOut), "subtitle") {
		t.Errorf("legenda não embutida no arquivo final. Streams: %s", string(probeOut))
	}
	_ = miOut
}
