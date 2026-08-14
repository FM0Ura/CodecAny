package ffmpeg

import (
	"reflect"
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
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1",
				"-c:v", "libx265", "-preset", "slow", "-crf", "20",
				"-pix_fmt", "yuv420p10le", "-x265-params", "open-gop=0",
				"-c:a", "copy", "-c:s", "copy",
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
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1",
				"-c:v", "libx265", "-preset", "fast", "-crf", "22",
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
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1",
				"-c:v", "libx265", "-preset", "slow",
				"-x265-params", "lossless=1:open-gop=0",
				"-c:a", "copy", "-c:s", "copy",
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
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1",
				"-c:v", "libsvtav1", "-preset", "6", "-crf", "30",
				"-pix_fmt", "yuv420p10le", "-svtav1-params", "tune=0",
				"-c:a", "copy", "-c:s", "copy",
			},
		},
		{
			name: "AV1 lossless",
			target: core.TargetSpec{
				VideoCodec:    "av1",
				VideoLossless: true,
			},
			want: []string{
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1",
				"-c:v", "libaom-av1", "-preset", "4", "-crf", "0",
				"-aom-params", "lossless=1",
				"-c:a", "copy", "-c:s", "copy",
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
				"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1",
				"-c:v", "libx264", "-preset", "medium", "-crf", "23",
				"-c:a", "copy", "-c:s", "copy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgs(tt.target)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
