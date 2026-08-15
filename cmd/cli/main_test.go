package main

import (
	"bytes"
	"strings"
	"testing"
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