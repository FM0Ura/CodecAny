package ffmpeg

import (
	"reflect"
	"strings"
	"testing"

	"github.com/FM0Ura/codecany/pkg/core"
)

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name   string
		target core.TargetSpec
		want   []string
	}{
		{
			name: "H265 lossy with defaults",
			target: core.TargetSpec{
				VideoCodec: "hevc",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libx265", "-preset", "slow", "-crf", "20",
				"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
				"-c:s", "copy",
			},
		},
		{
			name: "H265 lossy with custom CRF and preset",
			target: core.TargetSpec{
				VideoCodec:   "libx265",
				VideoCRF:     22,
				VideoPreset:  "fast",
				AudioCodec:   "aac",
				AudioBitrate: "192k",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libx265", "-preset", "fast", "-crf", "22",
				"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
				"-c:a", "aac", "-b:a", "192k", "-c:s", "copy",
			},
		},
		{
			name: "H265 lossless",
			target: core.TargetSpec{
				VideoCodec:    "hevc",
				VideoLossless: true,
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libx265", "-preset", "slow",
				"-x265-params", "lossless=1:open-gop=0",
				"-c:s", "copy",
			},
		},
		{
			name: "AV1 lossy with custom CRF and preset",
			target: core.TargetSpec{
				VideoCodec:  "av1",
				VideoCRF:    30,
				VideoPreset: "6",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libsvtav1", "-preset", "6", "-crf", "30",
				"-pix_fmt", "yuv420p10le", "-svtav1-params", "tune=0",
				"-c:s", "copy",
			},
		},
		{
			name: "AV1 lossless",
			target: core.TargetSpec{
				VideoCodec:    "av1",
				VideoLossless: true,
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libaom-av1", "-preset", "4", "-crf", "0",
				"-aom-params", "lossless=1",
				"-c:s", "copy",
			},
		},
		{
			name: "H264 legacy path unaffected",
			target: core.TargetSpec{
				VideoCodec:  "libx264",
				VideoCRF:    23,
				VideoPreset: "medium",
				AudioCodec:  "copy",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libx264", "-preset", "medium", "-crf", "23",
				"-c:a", "copy", "-c:s", "copy",
			},
		},
		{
			name: "H265 NVENC lossy with defaults",
			target: core.TargetSpec{
				VideoCodec:   "hevc",
				VideoHWAccel: "nvenc",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "hevc_nvenc", "-preset", "p5", "-rc", "vbr", "-cq", "23",
				"-pix_fmt", "yuv420p10le", "-c:s", "copy",
			},
		},
		{
			name: "H265 VAAPI lossy with defaults",
			target: core.TargetSpec{
				VideoCodec:   "hevc",
				VideoHWAccel: "vaapi",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "hevc_vaapi", "-c:s", "copy",
			},
		},
		{
			name: "H265 NVENC lossless",
			target: core.TargetSpec{
				VideoCodec:    "hevc",
				VideoHWAccel:  "nvenc",
				VideoLossless: true,
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "hevc_nvenc", "-preset", "lossless",
				"-c:s", "copy",
			},
		},
		{
			name: "H265 to MP4 with mov_text subtitles",
			target: core.TargetSpec{
				VideoCodec: "hevc",
				Container:  "mp4",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libx265", "-preset", "slow", "-crf", "20",
				"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
				"-c:s", "mov_text",
			},
		},
		{
			// Bug C: vendor de hwaccel não reconhecido (typo) deve cair
			// silenciosamente para o encoder por software (libx265), assim
			// como o caminho "sem hwaccel".
			name: "H265 unrecognized hwaccel falls back to software",
			target: core.TargetSpec{
				VideoCodec:   "hevc",
				VideoHWAccel: "nvidia",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libx265", "-preset", "slow", "-crf", "20",
				"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
				"-c:s", "copy",
			},
		},
		{
			// Bug C: vendor reconhecido mas sem suporte ao codec base pedido
			// (videotoolbox não tem encoder av1) também deve cair para
			// software, respeitando VideoLossless como o ramo "sem hwaccel".
			name: "AV1 videotoolbox unsupported falls back to software",
			target: core.TargetSpec{
				VideoCodec:   "av1",
				VideoHWAccel: "videotoolbox",
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
				"-c:v", "copy", "-c:v:0", "libsvtav1", "-preset", "5", "-crf", "28",
				"-pix_fmt", "yuv420p10le", "-svtav1-params", "tune=0",
				"-c:s", "copy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMP4 := tt.target.Container == "mp4"
			got := buildArgs(tt.target, isMP4, nil)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildArgs_DynamicAudio(t *testing.T) {
	target := core.TargetSpec{
		VideoCodec: "av1",
		AudioCodec: "flac",
	}
	// Input com mix de TrueHD (lossless), AAC (lossy) e DTS (heavy)
	audioCodecs := []string{"truehd", "aac", "dts"}

	want := []string{
		"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
		"-c:v", "copy", "-c:v:0", "libsvtav1", "-preset", "5", "-crf", "28",
		"-pix_fmt", "yuv420p10le", "-svtav1-params", "tune=0",
		"-c:a:0", "flac", "-c:a:1", "copy", "-c:a:2", "flac",
		"-c:s", "copy",
	}

	got := buildArgs(target, false, audioCodecs)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArgs() com áudio dinâmico = %v, want %v", got, want)
	}
}

// TestBuildVideoMapArgs cobre o Bug A: com capa (attached_pic) + vídeo real
// misturados, o vídeo real deve sempre virar o "-map" de índice de saída 0,
// e a(s) capa(s) devem ser mapeadas em seguida (para permanecerem em "copy"
// via o "-c:v copy" padrão de buildArgs).
func TestBuildVideoMapArgs(t *testing.T) {
	tests := []struct {
		name  string
		media core.MediaInfo
		want  []string
	}{
		{
			name:  "sem vídeo nenhum",
			media: core.MediaInfo{},
			want:  nil,
		},
		{
			name: "apenas vídeo real, sem capa",
			media: core.MediaInfo{
				HasVideo:         true,
				VideoStreamIndex: 0,
			},
			want: []string{"-map", "0:v:0"},
		},
		{
			name: "capa é o stream 0 no arquivo, vídeo real é o stream 1",
			media: core.MediaInfo{
				HasVideo:              true,
				VideoStreamIndex:      1,
				CoverArtStreamIndexes: []int{0},
			},
			// O vídeo real (índice de entrada 1) deve ser mapeado PRIMEIRO,
			// mesmo aparecendo depois da capa no container de origem, para
			// garantir que caia no índice de SAÍDA 0 (alvo de "-c:v:0").
			want: []string{"-map", "0:v:1", "-map", "0:v:0"},
		},
		{
			name: "arquivo só com capa (sem vídeo real)",
			media: core.MediaInfo{
				HasVideo:              false,
				CoverArtStreamIndexes: []int{0},
			},
			want: []string{"-map", "0:v:0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildVideoMapArgs(tt.media)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildVideoMapArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuildSubtitleMapArgs cobre o Bug B: ao gerar MP4, streams de legenda
// baseados em imagem (PGS/VobSub/DVB) devem ser descartados do mapeamento;
// fora do MP4, todos os streams devem ser mantidos.
func TestBuildSubtitleMapArgs(t *testing.T) {
	// Mix: 0=subrip (texto), 1=hdmv_pgs_subtitle (imagem), 2=ass (texto).
	codecs := []string{"subrip", "hdmv_pgs_subtitle", "ass"}

	t.Run("MP4 descarta apenas a legenda de imagem", func(t *testing.T) {
		got := buildSubtitleMapArgs(codecs, true)
		want := []string{"-map", "0:s:0", "-map", "0:s:2"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("buildSubtitleMapArgs(mp4) = %v, want %v", got, want)
		}
	})

	t.Run("não-MP4 mantém todos os streams", func(t *testing.T) {
		got := buildSubtitleMapArgs(codecs, false)
		want := []string{"-map", "0:s:0", "-map", "0:s:1", "-map", "0:s:2"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("buildSubtitleMapArgs(não-mp4) = %v, want %v", got, want)
		}
	})

	t.Run("todas as legendas são de imagem em MP4", func(t *testing.T) {
		got := buildSubtitleMapArgs([]string{"dvd_subtitle", "dvb_subtitle"}, true)
		if len(got) != 0 {
			t.Errorf("buildSubtitleMapArgs() = %v, want vazio", got)
		}
	})
}

// TestResolveHWAccelEncoder cobre o Bug C: a tabela única de vendors deve
// reconhecer combinações válidas e rejeitar vendors desconhecidos ou pares
// vendor/codec sem suporte (ex.: videotoolbox+av1).
func TestResolveHWAccelEncoder(t *testing.T) {
	tests := []struct {
		hw, codecBase  string
		wantEncoder    string
		wantRecognized bool
	}{
		{"nvenc", "hevc", "hevc_nvenc", true},
		{"cuda", "h264", "h264_nvenc", true},
		{"vaapi", "av1", "av1_vaapi", true},
		{"qsv", "hevc", "hevc_qsv", true},
		{"videotoolbox", "h264", "h264_videotoolbox", true},
		{"videotoolbox", "av1", "", false}, // sem suporte a av1
		{"nvidia", "hevc", "", false},      // vendor desconhecido (typo)
		{"", "hevc", "", false},
	}
	for _, tt := range tests {
		enc, recognized := resolveHWAccelEncoder(tt.hw, tt.codecBase)
		if enc != tt.wantEncoder || recognized != tt.wantRecognized {
			t.Errorf("resolveHWAccelEncoder(%q, %q) = (%q, %v), want (%q, %v)",
				tt.hw, tt.codecBase, enc, recognized, tt.wantEncoder, tt.wantRecognized)
		}
	}
}

// TestBuildTranscodeArgs_CoverArtPlusRealVideo cobre o Bug A de ponta a
// ponta: fonte com capa (stream de entrada 0) e vídeo real (stream de
// entrada 1) deve mapear o vídeo real primeiro (índice de saída 0, alvo de
// "-c:v:0") e a capa depois (permanece em "copy").
func TestBuildTranscodeArgs_CoverArtPlusRealVideo(t *testing.T) {
	media := core.MediaInfo{
		HasVideo:              true,
		VideoStreamIndex:      1,
		CoverArtStreamIndexes: []int{0},
		HasAudio:              true,
	}
	target := core.TargetSpec{VideoCodec: "hevc"}

	got := buildTranscodeArgs("in.mkv", "out.mkv", media, target)
	want := []string{
		"-i", "in.mkv",
		"-map", "0:v:1", "-map", "0:v:0",
		"-map", "0:a",
		"-map", "0:t?",
		"-map_metadata", "0",
		"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
		"-c:v", "copy", "-c:v:0", "libx265", "-preset", "slow", "-crf", "20",
		"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
		"-c:s", "copy",
		"out.mkv",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildTranscodeArgs() = %v, want %v", got, want)
	}
}

// TestBuildTranscodeArgs_MP4DropsImageSubtitles cobre o Bug B de ponta a
// ponta: ao gerar MP4 com um mix de legendas embutidas de texto e imagem,
// só a de imagem deve ser descartada do mapeamento.
func TestBuildTranscodeArgs_MP4DropsImageSubtitles(t *testing.T) {
	media := core.MediaInfo{
		HasVideo:         true,
		VideoStreamIndex: 0,
		SubtitleCodecs:   []string{"subrip", "hdmv_pgs_subtitle"},
	}
	target := core.TargetSpec{VideoCodec: "hevc", Container: "mp4"}

	got := buildTranscodeArgs("in.mkv", "out.mp4", media, target)
	want := []string{
		"-i", "in.mkv",
		"-map", "0:v:0",
		"-map", "0:s:0",
		"-map", "0:t?",
		"-map_metadata", "0",
		"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1",
		"-c:v", "copy", "-c:v:0", "libx265", "-preset", "slow", "-crf", "20",
		"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
		"-c:s", "mov_text",
		"out.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildTranscodeArgs() = %v, want %v", got, want)
	}
}

// TestBuildTranscodeArgs_UnrecognizedHWAccelOmitsFlag cobre o Bug C de ponta
// a ponta: um valor de VideoHWAccel não reconhecido não deve resultar em
// nenhuma flag "-hwaccel" no comando final, mesmo caindo para software.
func TestBuildTranscodeArgs_UnrecognizedHWAccelOmitsFlag(t *testing.T) {
	media := core.MediaInfo{HasVideo: true, VideoStreamIndex: 0}
	target := core.TargetSpec{VideoCodec: "hevc", VideoHWAccel: "nvidia"}

	got := buildTranscodeArgs("in.mkv", "out.mkv", media, target)
	for i, a := range got {
		if a == "-hwaccel" {
			t.Fatalf("buildTranscodeArgs() não deveria conter -hwaccel para vendor não reconhecido, got %v (índice %d)", got, i)
		}
	}
	// O encoder por software ainda deve ser usado.
	found := false
	for _, a := range got {
		if a == "libx265" {
			found = true
		}
	}
	if !found {
		t.Errorf("buildTranscodeArgs() esperava fallback para libx265, got %v", got)
	}
}

// TestBuildTranscodeArgs_RecognizedHWAccelKeepsFlag garante que o caso
// reconhecido continua emitindo "-hwaccel" com o alias correto (nvenc->cuda).
func TestBuildTranscodeArgs_RecognizedHWAccelKeepsFlag(t *testing.T) {
	media := core.MediaInfo{HasVideo: true, VideoStreamIndex: 0}
	target := core.TargetSpec{VideoCodec: "hevc", VideoHWAccel: "nvenc"}

	got := buildTranscodeArgs("in.mkv", "out.mkv", media, target)
	want := []string{"-hwaccel", "cuda"}
	if len(got) < 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("buildTranscodeArgs()[:2] = %v, want %v", got[:min(2, len(got))], want)
	}
}

// TestReadProgress_EmitsFractionsWhenDurationKnown garante que, com uma
// duração conhecida, a leitura de "-progress pipe:1" produz frações
// intermediárias (não só o 1.0 final de progress=end).
func TestReadProgress_EmitsFractionsWhenDurationKnown(t *testing.T) {
	input := strings.Join([]string{
		"frame=1",
		"out_time_ms=1000000",
		"progress=continue",
		"frame=2",
		"out_time_ms=5000000",
		"progress=continue",
		"frame=3",
		"out_time_ms=9000000",
		"progress=continue",
		"progress=end",
	}, "\n")

	out := make(chan float64, 32)
	readProgress(strings.NewReader(input), 10.0, out)
	close(out)

	var got []float64
	for v := range out {
		got = append(got, v)
	}
	if len(got) == 0 {
		t.Fatal("esperava ao menos uma fração de progresso")
	}
	foundIntermediate := false
	for _, v := range got[:len(got)-1] {
		if v > 0 && v < 1 {
			foundIntermediate = true
		}
	}
	if !foundIntermediate {
		t.Errorf("esperava frações intermediárias antes do progress=end, got %v", got)
	}
	if got[len(got)-1] != 1.0 {
		t.Errorf("último valor esperado 1.0 (progress=end), got %v", got[len(got)-1])
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("frações fora de ordem crescente: %v", got)
			break
		}
	}
}

// TestReadProgress_NoIntermediateFractionsWhenDurationZero é a guarda de
// regressão do bug relatado: quando durationSec é 0 (ex.: format.duration
// ausente e nenhum fallback disponível), NENHUMA fração intermediária deve
// chegar ao canal — só o 1.0 final de progress=end. É exatamente esse
// comportamento que fazia a UI da CLI parecer travada, só atualizando no
// início e no fim da conversão.
func TestReadProgress_NoIntermediateFractionsWhenDurationZero(t *testing.T) {
	input := strings.Join([]string{
		"frame=1",
		"out_time_ms=1000000",
		"progress=continue",
		"frame=2",
		"out_time_ms=5000000",
		"progress=continue",
		"progress=end",
	}, "\n")

	out := make(chan float64, 32)
	readProgress(strings.NewReader(input), 0, out)
	close(out)

	var got []float64
	for v := range out {
		got = append(got, v)
	}
	if len(got) != 1 || got[0] != 1.0 {
		t.Errorf("com durationSec=0 esperava só [1.0], got %v", got)
	}
}

// TestReadProgress_ClampsFractionRange garante que nenhuma fração fora de
// [0,1] é emitida (ex.: out_time momentaneamente maior que a duração
// probada, por imprecisão de duração estimada).
func TestReadProgress_ClampsFractionRange(t *testing.T) {
	input := strings.Join([]string{
		"out_time_ms=20000000", // 20s > duração (10s) -> ratio 2.0, fora de [0,1]
		"progress=continue",
		"progress=end",
	}, "\n")

	out := make(chan float64, 32)
	readProgress(strings.NewReader(input), 10.0, out)
	close(out)

	for v := range out {
		if v < 0 || v > 1 {
			t.Errorf("fração fora de [0,1]: %v", v)
		}
	}
}

func TestParseOutTime(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"00:00:00.000000", 0, false},
		{"01:02:03.456", 3723.456, false},
		{"00:25:12.219000000", 1512.219, false}, // formato de 9 dígitos visto no arquivo real (tags DURATION)
		{"abc", 0, true},
		{"00:01", 0, true}, // menos de 3 partes separadas por ":"
	}
	for _, c := range cases {
		got, err := parseOutTime(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseOutTime(%q) esperava erro, obteve %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseOutTime(%q) erro inesperado: %v", c.in, err)
			continue
		}
		if diff := got - c.want; diff > 0.001 || diff < -0.001 {
			t.Errorf("parseOutTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
