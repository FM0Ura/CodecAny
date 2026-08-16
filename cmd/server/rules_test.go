package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/FM0Ura/codecany/pkg/core"
)

// staticProber implementa core.MediaProber com uma resposta fixa (ou erro)
// configurável — fakeProber (router_test.go) só devolve MediaInfo{Path:
// path}, insuficiente para os testes de POST /api/rules/test que precisam
// controlar container/video_codec para exercitar casamento de regra.
type staticProber struct {
	info core.MediaInfo
	err  error
}

func (p staticProber) Probe(path string) (core.MediaInfo, error) { return p.info, p.err }

func TestHandleGetRules(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/rules")
	if err != nil {
		t.Fatalf("GET /api/rules: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got core.RuleFile
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0].Name != "h264 para av1" {
		t.Fatalf("rules inesperadas: %+v", got.Rules)
	}
	// Item escalar deve round-tripar como escalar JSON (não array de 1),
	// ver TestItemMarshalJSONRoundTrip em pkg/core — aqui confirmamos que o
	// contrato de API também se beneficia disso.
	if got.Rules[0].Match.Video.Codec.Values[0] != "h264" {
		t.Fatalf("match.video.codec inesperado: %+v", got.Rules[0].Match.Video.Codec)
	}
}

func TestHandlePutRulesValidPersistsAndHotReloads(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	newFile := core.RuleFile{
		Version: 1,
		Global: core.RuleFileGlobal{
			DefaultDriver: "ffmpeg",
			StagingDir:    ".codecany_tmp",
			SpaceSaving:   core.SpaceSavingPolicy{MinSavingPct: 15},
		},
		Rules: []core.Rule{
			{
				Name:   "av1 para hevc",
				Action: core.ActionConvert,
				Match: core.Match{
					Video: core.MatchVideo{Codec: core.Item{Values: []string{"av1"}}},
				},
				Convert: core.ConvertSpec{
					Video: &core.TargetSpecVideo{Codec: "hevc"},
				},
			},
		},
	}
	body, err := json.Marshal(newFile)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/rules", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/rules: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Persistiu em disco.
	diskBytes, err := os.ReadFile(ts.app.Config.RulesPath)
	if err != nil {
		t.Fatalf("ler rules.yaml: %v", err)
	}
	if !bytes.Contains(diskBytes, []byte("av1 para hevc")) {
		t.Fatalf("rules.yaml em disco não reflete a nova regra: %s", diskBytes)
	}

	// Hot-reload observável: POST /api/rules/test com uma mídia av1 agora
	// deve casar com a regra nova (que não existia na regra original,
	// h264->av1, fixada em writeFixtureRules).
	testBody, _ := json.Marshal(rulesTestRequest{
		MediaInfo: &core.MediaInfo{Container: "mkv", VideoCodec: "av1"},
	})
	testResp, err := http.Post(srv.URL+"/api/rules/test", "application/json", bytes.NewReader(testBody))
	if err != nil {
		t.Fatalf("POST /api/rules/test: %v", err)
	}
	defer testResp.Body.Close()
	if testResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", testResp.StatusCode)
	}
	var got rulesTestResponse
	if err := json.NewDecoder(testResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Outcome != "convert" {
		t.Fatalf("outcome = %q, want convert (regras deveriam ter sido recarregadas em runtime); describe_miss=%s", got.Outcome, got.DescribeMiss)
	}
	if got.TargetSpec.VideoCodec != "hevc" {
		t.Errorf("target_spec.video_codec = %q, want hevc", got.TargetSpec.VideoCodec)
	}
}

func TestHandlePutRulesInvalidLeavesFileUnchanged(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	before, err := os.ReadFile(ts.app.Config.RulesPath)
	if err != nil {
		t.Fatalf("ler rules.yaml antes: %v", err)
	}

	invalid := core.RuleFile{
		Version: 1,
		Global: core.RuleFileGlobal{
			DefaultDriver: "ffmpeg",
		},
		Rules: []core.Rule{}, // vazio: inválido (RuleFile.Validate)
	}
	body, _ := json.Marshal(invalid)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/rules", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/rules: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var errBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Error("esperava mensagem de erro clara no corpo")
	}

	after, err := os.ReadFile(ts.app.Config.RulesPath)
	if err != nil {
		t.Fatalf("ler rules.yaml depois: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("arquivo de regras em disco foi alterado por um PUT inválido:\nantes=%s\ndepois=%s", before, after)
	}
}

func TestHandleTestRuleWithMediaInfo(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	// Fixture (writeFixtureRules): regra única casa video.codec=h264 -> av1.
	body, _ := json.Marshal(rulesTestRequest{
		MediaInfo: &core.MediaInfo{Container: "mkv", VideoCodec: "h264"},
	})
	resp, err := http.Post(srv.URL+"/api/rules/test", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/rules/test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got rulesTestResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Outcome != "convert" {
		t.Fatalf("outcome = %q, want convert; describe_miss=%s", got.Outcome, got.DescribeMiss)
	}
	if got.TargetSpec.VideoCodec != "av1" {
		t.Errorf("target_spec.video_codec = %q, want av1", got.TargetSpec.VideoCodec)
	}
	if got.DescribeMiss == "" {
		t.Error("describe_miss não deveria ser vazio")
	}
}

func TestHandleTestRuleWithPath(t *testing.T) {
	ts := newTestServer(t)
	// Troca o prober por um controlável para este teste.
	ts.app.Prober = staticProber{info: core.MediaInfo{Container: "mov,mp4", VideoCodec: "hevc"}}
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	body, _ := json.Marshal(rulesTestRequest{Path: "/media/algum-arquivo.mp4"})
	resp, err := http.Post(srv.URL+"/api/rules/test", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/rules/test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got rulesTestResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Fixture só casa video.codec h264; hevc não casa com nenhuma regra.
	if got.Outcome != "skip_no_rule" {
		t.Fatalf("outcome = %q, want skip_no_rule (hevc não casa a fixture); describe_miss=%s", got.Outcome, got.DescribeMiss)
	}
}

func TestHandleTestRuleRequiresPathOrMediaInfo(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/rules/test", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST /api/rules/test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
