package ffmpeg

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/FM0Ura/codecany/pkg/core"
)

// Transcode implements core.TranscoderEngine usando o binário ffmpeg.
// Traduz o TargetSpec em flags nativas (seção 4.4) e parseia o progresso
// do -progress pipe: em tempo real (RF04).
type Transcode struct {
	bin string
}

// NewTranscode cria um transcoder usando o ffmpeg do PATH.
func NewTranscode() *Transcode { return &Transcode{bin: "ffmpeg"} }

// WithBin permite apontar para um binário específico.
func (t *Transcode) WithBin(bin string) *Transcode { t.bin = bin; return t }

// Name retorna o identificador do driver (RF04).
func (t *Transcode) Name() string { return "ffmpeg" }

func isHeavyLosslessAudio(codec string) bool {
	codec = strings.ToLower(codec)
	return codec == "truehd" ||
		strings.HasPrefix(codec, "dts") ||
		strings.HasPrefix(codec, "pcm_") ||
		codec == "lpcm"
}

// buildArgs monta os argumentos do ffmpeg a partir do TargetSpec.
func buildArgs(target core.TargetSpec, isMP4 bool, audioCodecs []string) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1"}

	if target.VideoCodec != "" {
		codec := target.VideoCodec

		if target.VideoHWAccel != "" {
			hw := strings.ToLower(target.VideoHWAccel)
			if codec == "hevc" {
				if hw == "nvenc" || hw == "cuda" {
					codec = "hevc_nvenc"
				} else if hw == "vaapi" {
					codec = "hevc_vaapi"
				} else if hw == "qsv" {
					codec = "hevc_qsv"
				} else if hw == "videotoolbox" {
					codec = "hevc_videotoolbox"
				} else {
					codec = "libx265"
				}
			} else if codec == "h264" {
				if hw == "nvenc" || hw == "cuda" {
					codec = "h264_nvenc"
				} else if hw == "vaapi" {
					codec = "h264_vaapi"
				} else if hw == "qsv" {
					codec = "h264_qsv"
				} else if hw == "videotoolbox" {
					codec = "h264_videotoolbox"
				} else {
					codec = "libx264"
				}
			} else if codec == "av1" {
				if hw == "nvenc" || hw == "cuda" {
					codec = "av1_nvenc"
				} else if hw == "vaapi" {
					codec = "av1_vaapi"
				} else if hw == "qsv" {
					codec = "av1_qsv"
				} else {
					codec = "libsvtav1"
				}
			}
		} else {
			// Mapeia codecs genéricos para encoders recomendados em software
			if codec == "hevc" {
				codec = "libx265"
			} else if codec == "av1" {
				if target.VideoLossless {
					codec = "libaom-av1"
				} else {
					codec = "libsvtav1"
				}
			}
		}

		if codec == "copy" {
			args = append(args, "-c:v", "copy")
		} else {
			args = append(args, "-c:v", "copy", "-c:v:0", codec)
		}

		if codec != "copy" {
			// Preset
			preset := target.VideoPreset
			if target.VideoLossless && strings.HasSuffix(codec, "_nvenc") {
				preset = ""
			}
			if preset == "" {
				if codec == "libx265" {
					preset = "slow"
				} else if codec == "libsvtav1" {
					preset = "5"
				} else if codec == "libaom-av1" {
					preset = "4"
				} else if strings.HasSuffix(codec, "_nvenc") {
					if !target.VideoLossless {
						preset = "p5"
					}
				}
			}
			if preset != "" {
				args = append(args, "-preset", preset)
			}

			// Lógica de Sem Perdas (Lossless) vs Com Perdas (Lossy)
			if target.VideoLossless {
				if codec == "libx265" {
					args = append(args, "-x265-params", "lossless=1:open-gop=0")
				} else if codec == "libaom-av1" {
					args = append(args, "-crf", "0", "-aom-params", "lossless=1")
				} else if strings.HasSuffix(codec, "_nvenc") {
					args = append(args, "-preset", "lossless")
				} else if strings.HasSuffix(codec, "_vaapi") {
					args = append(args, "-qp", "0")
				} else if strings.HasSuffix(codec, "_qsv") {
					args = append(args, "-global_quality", "0")
				} else {
					args = append(args, "-crf", "0")
				}
			} else {
				// Com perdas (Lossy)
				crf := target.VideoCRF
				if crf == 0 {
					if codec == "libx265" {
						crf = 20
					} else if codec == "libsvtav1" {
						crf = 28
					} else if strings.HasSuffix(codec, "_nvenc") {
						crf = 23
					}
				}
				if crf > 0 {
					if strings.HasSuffix(codec, "_nvenc") {
						args = append(args, "-rc", "vbr", "-cq", strconv.Itoa(crf))
					} else if strings.HasSuffix(codec, "_vaapi") || strings.HasSuffix(codec, "_qsv") {
						args = append(args, "-global_quality", strconv.Itoa(crf))
					} else if strings.HasSuffix(codec, "_videotoolbox") {
						args = append(args, "-q:v", strconv.Itoa(crf))
					} else {
						args = append(args, "-crf", strconv.Itoa(crf))
					}
				}

				// pix_fmt yuv420p10le para evitar color banding
				if codec == "libx265" || codec == "libsvtav1" || strings.HasSuffix(codec, "_nvenc") {
					args = append(args, "-pix_fmt", "yuv420p10le")
				}

				// Parâmetros específicos
				if codec == "libx265" {
					args = append(args, "-x265-params", "open-gop=0")
				} else if codec == "libsvtav1" {
					args = append(args, "-svtav1-params", "tune=0")
				}
			}
		}
	}

	if target.AudioCodec != "" {
		if target.AudioCodec == "flac" && len(audioCodecs) > 0 {
			for idx, codecName := range audioCodecs {
				if isHeavyLosslessAudio(codecName) {
					args = append(args, fmt.Sprintf("-c:a:%d", idx), "flac")
				} else {
					args = append(args, fmt.Sprintf("-c:a:%d", idx), "copy")
				}
			}
		} else {
			args = append(args, "-c:a", target.AudioCodec)
			if target.AudioBitrate != "" && target.AudioCodec != "copy" {
				args = append(args, "-b:a", target.AudioBitrate)
			}
		}
	}
	// Se nenhum codec de áudio foi especificado (nem pela regra, nem por
	// default), omite a flag -c:a e deixa o ffmpeg escolher um encoder
	// compatível com o container de saída. Forçar "copy" aqui falha quando o
	// codec de origem é incompatível com o container alvo (ex.: DTS/TrueHD → MP4).

	// Sempre copia legendas (com mapping específico se for MP4)
	if isMP4 {
		args = append(args, "-c:s", "mov_text")
	} else {
		args = append(args, "-c:s", "copy")
	}

	return args
}

// Transcode executa o ffmpeg e emite progresso (0..1) no canal retornado.
func (t *Transcode) Transcode(input, output string, media core.MediaInfo, target core.TargetSpec) (<-chan float64, error) {
	// Monta mapeamento completo dos fluxos para evitar perda de faixas secundárias
	var args []string
	if target.VideoHWAccel != "" {
		hw := strings.ToLower(target.VideoHWAccel)
		if hw == "nvenc" {
			hw = "cuda"
		}
		args = append(args, "-hwaccel", hw)
	}
	args = append(args, "-i", input)

	for _, sub := range media.SubtitlePaths {
		args = append(args, "-i", sub)
	}

	if media.HasVideo {
		args = append(args, "-map", "0:v")
	}
	if media.HasAudio {
		args = append(args, "-map", "0:a")
	}
	args = append(args, "-map", "0:s?", "-map", "0:t?")

	for idx := range media.SubtitlePaths {
		args = append(args, "-map", fmt.Sprintf("%d:s", idx+1))
	}

	// Preserva todos os metadados globais do arquivo de entrada (seção 10.1 do guia)
	args = append(args, "-map_metadata", "0")

	isMP4 := target.Container == "mp4" || strings.HasSuffix(strings.ToLower(output), ".mp4")
	args = append(args, buildArgs(target, isMP4, media.AudioCodecs)...)
	args = append(args, output)

	cmd := exec.Command(t.bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	prog := make(chan float64, 128)
	go func() {
		defer close(prog)
		sc := bufio.NewScanner(stdout)
		outTime := 0.0
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "out_time_ms=") {
				v, err := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_ms="), 64)
				if err == nil {
					outTime = v / 1e6
				}
			} else if strings.HasPrefix(line, "out_time=") {
				if d, err := parseOutTime(strings.TrimPrefix(line, "out_time=")); err == nil {
					outTime = d
				}
			} else if line == "progress=end" {
				prog <- 1.0
			}
			if media.DurationSec > 0 {
				if r := outTime / media.DurationSec; r >= 0 && r <= 1 {
					prog <- r
				}
			}
		}
		cmd.Wait()
	}()

	return prog, nil
}

// parseOutTime interpreta timcodes HH:MM:SS.microseconds.
func parseOutTime(s string) (float64, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) > 1 {
		// mantém micros
		micro := parts[1]
		s = parts[0] + "." + micro[:min(len(micro), 3)]
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return 0, fmt.Errorf("formato out_time inválido: %s", s)
	}
	h, _ := strconv.ParseFloat(hms[0], 64)
	mint, _ := strconv.ParseFloat(hms[1], 64)
	sec, _ := strconv.ParseFloat(strings.Split(hms[2], ".")[0], 64)
	micro := 0.0
	if len(parts) > 1 {
		micro, _ = strconv.ParseFloat("0."+parts[1], 64)
	}
	return h*3600 + mint*60 + sec + micro, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ core.TranscoderEngine = (*Transcode)(nil)
