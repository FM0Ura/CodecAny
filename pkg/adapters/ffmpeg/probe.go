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
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		BitRate   string `json:"bit_rate"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
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
	mi.DurationSec = parseDuration(r.Format.Duration)
	for _, s := range r.Streams {
		switch s.CodecType {
		case "video":
			if !mi.HasVideo {
				mi.HasVideo = true
				mi.VideoCodec = s.CodecName
				mi.VideoBitrate = parseBitrate(s.BitRate)
				mi.Width, mi.Height = s.Width, s.Height
			}
		case "audio":
			mi.HasAudio = true
			mi.AudioCodecs = append(mi.AudioCodecs, s.CodecName)
		}
	}

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

var _ core.MediaProber = (*Prober)(nil)
