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

// buildArgs monta os argumentos do ffmpeg a partir do TargetSpec.
func buildArgs(target core.TargetSpec) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1"}

	if target.VideoCodec != "" {
		args = append(args, "-c:v", target.VideoCodec)
		if target.VideoCodec != "copy" {
			if target.VideoCRF > 0 {
				args = append(args, "-crf", strconv.Itoa(target.VideoCRF))
			}
			if target.VideoPreset != "" {
				args = append(args, "-preset", target.VideoPreset)
			}
		}
	}
	if target.AudioCodec != "" {
		args = append(args, "-c:a", target.AudioCodec)
		if target.AudioBitrate != "" && target.AudioCodec != "copy" {
			args = append(args, "-b:a", target.AudioBitrate)
		}
	}
	return args
}

// Transcode executa o ffmpeg e emite progresso (0..1) no canal retornado.
func (t *Transcode) Transcode(input, output string, media core.MediaInfo, target core.TargetSpec) (<-chan float64, error) {
	args := append([]string{"-i", input}, buildArgs(target)...)
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
