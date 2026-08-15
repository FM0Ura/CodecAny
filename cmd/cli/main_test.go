package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FM0Ura/codecany/pkg/core"
)

func TestProgressBarUpdate(t *testing.T) {
	var buf bytes.Buffer
	b := newProgressBar(&buf)
	b.update("12345678", "/tmp/data/arquivoo.mkv", 42.0)
	line := buf.String()

	if !strings.HasPrefix(line, "\r[12345678]") {
		t.Errorf("barra deveria começar com \\r + id; obteve: %q", line)
	}
	if !strings.Contains(line, "arquivoo.mkv") {
		t.Errorf("barra deveria conter o nome do arquivo; obteve: %q", line)
	}
	if !strings.Contains(line, " 42.0000%") {
		t.Errorf("barra deveria conter o percentual 42; obteve: %q", line)
	}
}

func TestProgressBarClamps(t *testing.T) {
	var buf bytes.Buffer
	b := newProgressBar(&buf)
	b.update("12345678", "a.mkv", 150.0) // >100 deve virar 100
	out := buf.String()
	if !strings.Contains(out, "100.0000%") {
		t.Errorf("percentual deveria ser limitado a 100; obteve: %q", out)
	}
	buf.Reset()
	b.update("12345678", "a.mkv", -10.0) // <0 deve virar 0
	if !strings.Contains(buf.String(), "  0.0000%") {
		t.Errorf("percentual deveria ser limitado a 0; obteve: %q", buf.String())
	}
}

func TestHumanBytesFormatsMBAndGB(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1536, "1.50 KB"},                // 1.5 KB
		{640 * 1024 * 1024, "640.00 MB"}, // 640 MB
		{1524713390, "1.42 GB"},
	}
	for _, c := range cases {
		got := humanBytes(c.bytes)
		if got != c.want {
			t.Errorf("humanBytes(%d) = %q, esperado %q", c.bytes, got, c.want)
		}
	}
}

func TestPrintApprovalSummaryFormatsFields(t *testing.T) {
	var buf bytes.Buffer
	ev := core.JobEvent{
		Kind:        core.EventJobAwaitingApproval,
		JobID:       "1a418284-aaaa-bbbb-cccc-ddddeeeeffff",
		FilePath:    "/videos/The Testament of Sister New Devil.mkv",
		TargetCodec: "av1",
		Metrics: core.SizeMetrics{
			OriginalSizeBytes:   1524000000,
			ConvertedSizeBytes:  671350000,
			SavedBytes:          852650000,
			CompressionRatioPct: 55.94,
		},
	}
	printApprovalSummary(ev, &buf)
	out := buf.String()

	for _, want := range []string{
		"[1a418284]",
		"The Testament of Sister New Devil.mkv",
		"AV1",
		"55.94%",
		"[y/N]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("resumo deveria conter %q; obteve: %s", want, out)
		}
	}
}

func TestFormatStagedTableColumns(t *testing.T) {
	var buf bytes.Buffer
	jobs := []*core.Job{
		{
			ID:   "1a418284-aaaa-bbbb-cccc-ddddeeeeffff",
			Path: "/videos/movie1.mkv",
			SizeMetrics: core.SizeMetrics{
				OriginalSizeBytes:   1524000000,
				ConvertedSizeBytes:  671000000,
				CompressionRatioPct: 56.18,
			},
		},
		{
			ID:   "fe3311ab-aaaa-bbbb-cccc-ddddeeeeffff",
			Path: "/videos/movie2.mkv",
			SizeMetrics: core.SizeMetrics{
				OriginalSizeBytes:   980000000,
				ConvertedSizeBytes:  810000000,
				CompressionRatioPct: 17.34,
			},
		},
	}
	formatStagedTable(jobs, &buf)
	out := buf.String()

	for _, want := range []string{
		"ID", "Arquivo", "Original", "Convertido", "Economia",
		"1a418284", "movie1.mkv", "56.18%",
		"fe3311ab", "movie2.mkv", "17.34%",
		"-approve " + jobs[0].ID,
		"-approve " + jobs[1].ID,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tabela deveria conter %q; obteve:\n%s", want, out)
		}
	}
}
