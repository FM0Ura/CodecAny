package core

import (
	"path/filepath"
	"testing"
)

func TestExampleRulesParse(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "rules.yaml")
	r, err := NewRulesEngine(path)
	if err != nil {
		t.Fatalf("rules de exemplo não deve falhar ao carregar: %v", err)
	}
	if r.Global().SpaceSaving.MinSavingPct != 15 {
		t.Errorf("min_saving_pct esperado 15, obteve %v", r.Global().SpaceSaving.MinSavingPct)
	}
	outcome, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", AudioCodecs: []string{"ac3"}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Errorf("MKV H264 deveria casar a 1ª regra, obteve %v", outcome)
	}
}
