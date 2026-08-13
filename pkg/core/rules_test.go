package core

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRules grava um arquivo de regras temporário e devolve o caminho.
func writeRules(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const baseRules = `
version: 1
global:
  staging_dir: /tmp/codecany-test-staging
  space_saving:
    min_saving_pct: 15
  defaults:
    video: { codec: hevc, crf: 22, preset: slow }
    audio: { codec: copy }
rules:
  - name: "h264 -> hevc"
    match:
      container: mkv
      video: { codec: h264 }
    convert:
      video: { codec: hevc, preset: medium }
`

func TestRulesEvaluateFirstMatch(t *testing.T) {
	r := mustEngine(t, baseRules)
	mi := MediaInfo{Container: "mkv", VideoCodec: "h264", AudioCodecs: []string{"aac"}}
	outcome, spec, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("esperava convert, obteve %v", outcome)
	}
	if spec.VideoCodec != "hevc" {
		t.Errorf("esperava hevc, obteve %s", spec.VideoCodec)
	}
	if spec.VideoPreset != "medium" {
		t.Errorf("preset da regra deveria vencer defaults; obteve %s", spec.VideoPreset)
	}
	if spec.VideoCRF != 22 {
		t.Errorf("crf do default deveria preencher; obteve %d", spec.VideoCRF)
	}
}

func TestRulesEvaluateContainerNoMatch(t *testing.T) {
	r := mustEngine(t, baseRules)
	mi := MediaInfo{Container: "mp4", VideoCodec: "h264"}
	outcome, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("esperava SkipNoRule, obteve %v", outcome)
	}
}

func TestRulesSkipAction(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "av1 skip"
    match:
      video: { codec: av1 }
    action: skip
`
	r := mustEngine(t, content)
	mi := MediaInfo{Container: "mkv", VideoCodec: "av1"}
	outcome, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkip {
		t.Fatalf("esperava skip, obteve %v", outcome)
	}
}

func TestRulesAudioAnyMatch(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "audio ok"
    match:
      container: mkv
      audio: { codec: [aac, ac3] }
`
	r := mustEngine(t, content)
	mi := MediaInfo{Container: "mkv", AudioCodecs: []string{"ac3"}}
	outcome, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("áudio ac3 deveria casar lista [aac,ac3]; obteve %v", outcome)
	}
}

func TestRulesJSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	content := `{"version":1,"global":{"staging_dir":"/tmp/xs"},"rules":[{"name":"x","match":{"video":{"codec":"h264"}},"convert":{"video":{"codec":"hevc"}}}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRulesEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	outcome, spec, err := r.Evaluate(MediaInfo{Container: "any", VideoCodec: "h264"})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert || spec.VideoCodec != "hevc" {
		t.Fatalf("JSON rules falhou: %v %+v", outcome, spec)
	}
}

func mustEngine(t *testing.T, content string) *RulesEngine {
	t.Helper()
	r, err := NewRulesEngine(writeRules(t, content))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
