// Package ffmpeg é o adapter padrão do CodecAny baseado no FFmpeg/FFprobe
// (seção 2, /pkg/adapters/ffmpeg). Implementa MediaProber e TranscoderEngine.
package ffmpeg

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FM0Ura/codecany/pkg/core"
)

// Prober implementa core.MediaProber usando o binário ffprobe.
type Prober struct {
	bin string
}

// NewProber cria um prober usando o ffprobe do PATH.
func NewProber() *Prober { return &Prober{bin: "ffprobe"} }

// WithBin permite apontar para um binário específico.
func (p *Prober) WithBin(bin string) *Prober { p.bin = bin; return p }

// ffprobeResult espelha a porção relevante do JSON emitido pelo ffprobe.
type ffprobeResult struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`     // fallback: estimativa via size/bit_rate
		BitRate    string `json:"bit_rate"` // fallback: estimativa via size/bit_rate
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		BitRate   string `json:"bit_rate"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"` // fallback: duração do stream (nem sempre preenchida)
		Tags      struct {
			// DURATION/DURATION-eng são tags no formato Matroska
			// "HH:MM:SS.nnnnnnnnn" — comuns em MKVs produzidos por mkvmerge
			// quando nem format.duration nem streams[].duration numérico
			// vêm preenchidos.
			Duration    string `json:"DURATION"`
			DurationEng string `json:"DURATION-eng"`
		} `json:"tags"`
		Disposition struct {
			// AttachedPic é 1 quando o stream de vídeo é, na verdade, uma
			// imagem anexada (capa de álbum/thumbnail) e não vídeo de
			// movimento real — ffprobe reporta isso via disposition.
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

// Probe extrai metadados técnicos via ffprobe (RF02).
func (p *Prober) Probe(path string) (core.MediaInfo, error) {
	cmd := exec.Command(p.bin,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return core.MediaInfo{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	var r ffprobeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return core.MediaInfo{}, fmt.Errorf("parse ffprobe: %w", err)
	}
	mi := core.MediaInfo{Path: path, Container: r.Format.FormatName}
	formatDuration := parseDuration(r.Format.Duration)
	// videoIdx conta TODOS os streams de tipo vídeo (incluindo capas
	// attached_pic), pois é esse o espaço de índices usado pelo seletor
	// ffmpeg "0:v:N" — precisa bater com o índice armazenado abaixo.
	videoIdx := 0
	var videoStreamDuration float64
	for _, s := range r.Streams {
		switch s.CodecType {
		case "video":
			if s.Disposition.AttachedPic == 1 {
				// Capa de álbum/thumbnail embutida: não é vídeo real, então
				// não deve virar mi.VideoCodec/Width/Height (RF02 — bug de
				// mis-seleção de stream). Guardamos o índice para que
				// transcode.go possa mapeá-la separadamente e copiá-la
				// sem recodificar.
				mi.CoverArtStreamIndexes = append(mi.CoverArtStreamIndexes, videoIdx)
			} else if !mi.HasVideo {
				// Primeiro stream de vídeo "real" (não attached_pic)
				// encontrado — é esse que representa o conteúdo do filme
				// para fins de casamento de regras e recodificação.
				mi.HasVideo = true
				mi.VideoCodec = s.CodecName
				mi.VideoBitrate = parseBitrate(s.BitRate)
				mi.Width, mi.Height = s.Width, s.Height
				mi.VideoStreamIndex = videoIdx
				// Duração do MESMO stream, usada como fallback quando
				// format.duration vem vazio/"N/A" (ver mi.DurationSec abaixo).
				videoStreamDuration = firstPositiveDuration(
					parseDuration(s.Duration),
					streamTagDuration(s.Tags.Duration, s.Tags.DurationEng),
				)
			}
			videoIdx++
		case "audio":
			mi.HasAudio = true
			mi.AudioCodecs = append(mi.AudioCodecs, s.CodecName)
		case "subtitle":
			mi.SubtitleCodecs = append(mi.SubtitleCodecs, s.CodecName)
		}
	}
	// Se TODOS os streams de vídeo forem attached_pic (arquivo só com capa,
	// sem vídeo de movimento), mi.HasVideo permanece false: uma capa isolada
	// não é conteúdo de vídeo real para fins de casamento de regras.

	// mi.DurationSec: format.duration é o caminho feliz (mais confiável),
	// mas alguns containers (ex.: MKV remuxado/fansub via mkvmerge) deixam
	// esse campo vazio/"N/A". Sem duração, o cálculo de progresso do
	// transcode (out_time/duration) nunca produz frações intermediárias —
	// só o "1.0" final de progress=end chega, dando a impressão de que a
	// conversão trava sem feedback. Por isso a cadeia de fallback:
	// duração do stream de vídeo real → tags Matroska DURATION/DURATION-eng
	// desse stream → estimativa via tamanho do arquivo / bitrate global.
	mi.DurationSec = firstPositiveDuration(
		formatDuration,
		videoStreamDuration,
		estimateDurationFromSizeBitrate(r.Format.Size, r.Format.BitRate),
	)

	base := strings.TrimSuffix(path, filepath.Ext(path))
	dir := filepath.Dir(path)
	baseNameLower := strings.ToLower(filepath.Base(base))

	if files, err := os.ReadDir(dir); err == nil {
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			nameLower := strings.ToLower(name)
			if strings.HasPrefix(nameLower, baseNameLower) {
				ext := filepath.Ext(nameLower)
				if ext == ".srt" || ext == ".vtt" || ext == ".ass" {
					rem := strings.TrimPrefix(nameLower, baseNameLower)
					if rem == ext || (strings.HasPrefix(rem, ".") && strings.HasSuffix(rem, ext)) {
						mi.SubtitlePaths = append(mi.SubtitlePaths, filepath.Join(dir, name))
					}
				}
			}
		}
	}
	sort.Strings(mi.SubtitlePaths)

	return mi, nil
}

func parseBitrate(s string) int64 {
	if s == "" || s == "N/A" {
		return 0
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func parseDuration(s string) float64 {
	if s == "" || s == "N/A" {
		return 0
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

// firstPositiveDuration retorna a primeira duração > 0 dentre os candidatos,
// na ordem de prioridade (mais confiável primeiro) dada pelo chamador.
func firstPositiveDuration(candidates ...float64) float64 {
	for _, d := range candidates {
		if d > 0 {
			return d
		}
	}
	return 0
}

// streamTagDuration extrai a duração (em segundos) das tags Matroska
// "DURATION"/"DURATION-eng" de um stream, formato "HH:MM:SS.nnnnnnnnn" usado
// por mkvmerge — comum em MKVs remuxados/fansub onde nem format.duration nem
// streams[].duration numérico vêm preenchidos. Reaproveita parseOutTime
// (transcode.go, mesmo pacote), que já interpreta esse timecode.
func streamTagDuration(tagDuration, tagDurationEng string) float64 {
	for _, s := range []string{tagDuration, tagDurationEng} {
		if s == "" {
			continue
		}
		if d, err := parseOutTime(s); err == nil && d > 0 {
			return d
		}
	}
	return 0
}

// estimateDurationFromSizeBitrate estima a duração (em segundos) a partir do
// tamanho total do arquivo e do bit_rate global de "ffprobe -show_format".
// Último recurso quando format.duration, a duração do stream de vídeo real e
// suas tags DURATION/DURATION-eng estão todos ausentes.
func estimateDurationFromSizeBitrate(sizeStr, bitRateStr string) float64 {
	size := parseBitrate(sizeStr) // reaproveita o parser de inteiro existente
	bitRate := parseBitrate(bitRateStr)
	if size <= 0 || bitRate <= 0 {
		return 0
	}
	return float64(size*8) / float64(bitRate)
}

var _ core.MediaProber = (*Prober)(nil)
