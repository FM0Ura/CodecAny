package ffmpeg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFakeFFprobe cria um binário fake de ffprobe (um script shell) que
// ignora todos os argumentos recebidos e imprime o JSON fornecido em stdout,
// simulando a saída de "ffprobe -show_format -show_streams -print_format
// json". Permite testar Probe() sem depender de um binário ffprobe real.
func writeFakeFFprobe(t *testing.T, jsonOutput string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script fake de ffprobe requer shell POSIX")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake_ffprobe.sh")
	content := "#!/bin/sh\ncat <<'FFPROBE_JSON_EOF'\n" + jsonOutput + "\nFFPROBE_JSON_EOF\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		t.Fatalf("escrever fake ffprobe: %v", err)
	}
	return scriptPath
}

// TestProbe_SkipsAttachedPicForRealVideo cobre o Bug A: quando o primeiro
// stream de vídeo do container é uma capa (disposition.attached_pic == 1),
// Probe() não deve tratá-lo como o vídeo real — deve pular para o próximo
// stream de vídeo não-attached_pic e registrar a capa em
// CoverArtStreamIndexes.
func TestProbe_SkipsAttachedPicForRealVideo(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "120.5"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "mjpeg",
				"width": 600,
				"height": 800,
				"disposition": {"attached_pic": 1}
			},
			{
				"codec_type": "video",
				"codec_name": "h264",
				"bit_rate": "5000000",
				"width": 1920,
				"height": 1080,
				"disposition": {"attached_pic": 0}
			},
			{
				"codec_type": "audio",
				"codec_name": "aac"
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}

	if !mi.HasVideo {
		t.Fatal("esperava HasVideo=true (stream de vídeo real presente)")
	}
	if mi.VideoCodec != "h264" {
		t.Errorf("esperava VideoCodec=h264 (ignorando a capa mjpeg), obteve %q", mi.VideoCodec)
	}
	if mi.Width != 1920 || mi.Height != 1080 {
		t.Errorf("esperava resolução 1920x1080 do vídeo real, obteve %dx%d", mi.Width, mi.Height)
	}
	if mi.VideoStreamIndex != 1 {
		t.Errorf("esperava VideoStreamIndex=1 (capa é o índice 0), obteve %d", mi.VideoStreamIndex)
	}
	if len(mi.CoverArtStreamIndexes) != 1 || mi.CoverArtStreamIndexes[0] != 0 {
		t.Errorf("esperava CoverArtStreamIndexes=[0], obteve %v", mi.CoverArtStreamIndexes)
	}
}

// TestProbe_OnlyCoverArtNoRealVideo cobre o caso extremo do Bug A: se TODOS
// os streams de vídeo forem attached_pic, não há vídeo "real" — HasVideo
// deve permanecer false, mas a capa ainda deve aparecer em
// CoverArtStreamIndexes para ser preservada no transcode.
func TestProbe_OnlyCoverArtNoRealVideo(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "mp3", "duration": "200.0"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "mjpeg",
				"width": 500,
				"height": 500,
				"disposition": {"attached_pic": 1}
			},
			{
				"codec_type": "audio",
				"codec_name": "mp3"
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "song.mp3"))
	if err != nil {
		t.Fatal(err)
	}

	if mi.HasVideo {
		t.Errorf("esperava HasVideo=false (só existe capa, sem vídeo real), obteve VideoCodec=%q", mi.VideoCodec)
	}
	if len(mi.CoverArtStreamIndexes) != 1 || mi.CoverArtStreamIndexes[0] != 0 {
		t.Errorf("esperava CoverArtStreamIndexes=[0], obteve %v", mi.CoverArtStreamIndexes)
	}
}

// TestProbe_CapturesSubtitleCodecs cobre o Bug B: Probe() deve capturar, em
// ordem, o codec_name de cada stream de legenda embutido no container, para
// que transcode.go possa decidir quais podem virar mov_text e quais (as
// baseadas em imagem) precisam ser descartadas ao gerar MP4.
func TestProbe_CapturesSubtitleCodecs(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "60.0"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1280,
				"height": 720,
				"disposition": {"attached_pic": 0}
			},
			{
				"codec_type": "subtitle",
				"codec_name": "subrip"
			},
			{
				"codec_type": "subtitle",
				"codec_name": "hdmv_pgs_subtitle"
			},
			{
				"codec_type": "subtitle",
				"codec_name": "ass"
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"subrip", "hdmv_pgs_subtitle", "ass"}
	if len(mi.SubtitleCodecs) != len(want) {
		t.Fatalf("esperava %d codecs de legenda, obteve %v", len(want), mi.SubtitleCodecs)
	}
	for i, c := range want {
		if mi.SubtitleCodecs[i] != c {
			t.Errorf("SubtitleCodecs[%d] = %q, want %q", i, mi.SubtitleCodecs[i], c)
		}
	}
}

// TestProbe_NoAttachedPicUnaffected garante que o comportamento anterior
// (sem capas) continua funcionando: o primeiro stream de vídeo vira o vídeo
// principal e VideoStreamIndex é 0.
func TestProbe_NoAttachedPicUnaffected(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "mov,mp4,m4a,3gp,3g2,mj2", "duration": "45.2"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "hevc",
				"bit_rate": "8000000",
				"width": 3840,
				"height": 2160,
				"disposition": {"attached_pic": 0}
			},
			{
				"codec_type": "audio",
				"codec_name": "eac3"
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mp4"))
	if err != nil {
		t.Fatal(err)
	}

	if !mi.HasVideo || mi.VideoCodec != "hevc" {
		t.Errorf("esperava vídeo hevc, obteve HasVideo=%v VideoCodec=%q", mi.HasVideo, mi.VideoCodec)
	}
	if mi.VideoStreamIndex != 0 {
		t.Errorf("esperava VideoStreamIndex=0, obteve %d", mi.VideoStreamIndex)
	}
	if len(mi.CoverArtStreamIndexes) != 0 {
		t.Errorf("esperava CoverArtStreamIndexes vazio, obteve %v", mi.CoverArtStreamIndexes)
	}
	if mi.Width != 3840 || mi.Height != 2160 {
		t.Errorf("esperava resolução 3840x2160, obteve %dx%d", mi.Width, mi.Height)
	}
}

// TestProbe_DurationUsesFormatDurationWhenPresent é o teste de regressão do
// caminho feliz: quando format.duration vem preenchido, ele prevalece mesmo
// que streams[].duration/tags divirjam — o fallback não deve sobrepor a
// fonte mais confiável.
func TestProbe_DurationUsesFormatDurationWhenPresent(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "120.5"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"duration": "999.0",
				"disposition": {"attached_pic": 0}
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if mi.DurationSec != 120.5 {
		t.Errorf("esperava DurationSec=120.5 (format.duration), obteve %v", mi.DurationSec)
	}
}

// TestProbe_DurationFallsBackToVideoStreamDuration cobre o bug relatado:
// format.duration ausente/"N/A" (comum em MKV remuxado) deve cair para a
// duração numérica do stream de vídeo real.
func TestProbe_DurationFallsBackToVideoStreamDuration(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "N/A"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"duration": "125.033000",
				"disposition": {"attached_pic": 0}
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if mi.DurationSec != 125.033 {
		t.Errorf("esperava DurationSec=125.033 (fallback streams[].duration), obteve %v", mi.DurationSec)
	}
}

// TestProbe_DurationFallbackIgnoresAttachedPicStream garante que o fallback
// usa a duração do stream de vídeo REAL (mesmo índice de VideoStreamIndex),
// não a de uma capa/attached_pic que apareça antes dele no container.
func TestProbe_DurationFallbackIgnoresAttachedPicStream(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "N/A"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "mjpeg",
				"width": 600,
				"height": 800,
				"duration": "0.04",
				"disposition": {"attached_pic": 1}
			},
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"duration": "200.0",
				"disposition": {"attached_pic": 0}
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if mi.DurationSec != 200.0 {
		t.Errorf("esperava DurationSec=200.0 (do vídeo real, ignorando a capa), obteve %v", mi.DurationSec)
	}
}

// TestProbe_DurationFallsBackToStreamTags cobre o caso descoberto no arquivo
// real de amostra do ambiente (mkvmerge): nem format.duration nem
// streams[].duration numérico vêm preenchidos, só a tag Matroska
// DURATION-eng no formato "HH:MM:SS.nnnnnnnnn".
func TestProbe_DurationFallsBackToStreamTags(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "N/A"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"disposition": {"attached_pic": 0},
				"tags": {"DURATION-eng": "00:25:12.219000000"}
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	const want = 1512.219
	if diff := mi.DurationSec - want; diff > 0.001 || diff < -0.001 {
		t.Errorf("esperava DurationSec≈%v (fallback tags DURATION-eng), obteve %v", want, mi.DurationSec)
	}
}

// TestProbe_DurationFallsBackToSizeBitrate cobre o último nível de fallback:
// nenhuma das fontes por stream disponível, só format.size/format.bit_rate.
func TestProbe_DurationFallsBackToSizeBitrate(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "N/A", "size": "125000000", "bit_rate": "1000000"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"disposition": {"attached_pic": 0}
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	const want = 1000.0 // 125_000_000*8 / 1_000_000
	if mi.DurationSec != want {
		t.Errorf("esperava DurationSec=%v (fallback size/bit_rate), obteve %v", want, mi.DurationSec)
	}
}

// TestProbe_DurationZeroWhenAllSourcesMissing documenta o degrade seguro:
// sem nenhuma fonte disponível, DurationSec fica 0 (não é regressão).
func TestProbe_DurationZeroWhenAllSourcesMissing(t *testing.T) {
	fakeJSON := `{
		"format": {"format_name": "matroska,webm", "duration": "N/A"},
		"streams": [
			{
				"codec_type": "video",
				"codec_name": "h264",
				"width": 1920,
				"height": 1080,
				"disposition": {"attached_pic": 0}
			}
		]
	}`
	bin := writeFakeFFprobe(t, fakeJSON)

	mi, err := NewProber().WithBin(bin).Probe(filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if mi.DurationSec != 0 {
		t.Errorf("esperava DurationSec=0 (nenhuma fonte disponível), obteve %v", mi.DurationSec)
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"", 0},
		{"N/A", 0},
		{"0", 0},
		{"120.5", 120.5},
		{"1512.219000", 1512.219},
	}
	for _, c := range cases {
		if got := parseDuration(c.in); got != c.want {
			t.Errorf("parseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEstimateDurationFromSizeBitrate(t *testing.T) {
	cases := []struct {
		size, bitRate string
		want          float64
	}{
		{"", "1000000", 0},
		{"125000000", "", 0},
		{"N/A", "1000000", 0},
		{"125000000", "1000000", 1000},
	}
	for _, c := range cases {
		if got := estimateDurationFromSizeBitrate(c.size, c.bitRate); got != c.want {
			t.Errorf("estimateDurationFromSizeBitrate(%q, %q) = %v, want %v", c.size, c.bitRate, got, c.want)
		}
	}
}
