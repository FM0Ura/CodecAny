package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	outcome, spec, ruleName, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("esperava convert, obteve %v", outcome)
	}
	if ruleName != "h264 -> hevc" {
		t.Errorf("esperava regra 'h264 -> hevc', obteve %q", ruleName)
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
	outcome, _, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("esperava SkipNoRule, obteve %v", outcome)
	}
}

func TestRulesDescribeMissAllCriteria(t *testing.T) {
	r := mustEngine(t, baseRules) // regra "h264 -> hevc": container mkv, video h264
	mi := MediaInfo{Container: "mov", VideoCodec: "hevc", AudioCodecs: []string{"aac"}}
	got := r.DescribeMiss(mi)
	for _, want := range []string{"h264 -> hevc:", "container(media=mov, esperado=[mkv])", "video.codec(media=hevc, esperado=[h264])"} {
		if !strings.Contains(got, want) {
			t.Errorf("DescribeMiss deveria conter %q; obteve: %s", want, got)
		}
	}
}

func TestRulesDescribeMissRuleMatches(t *testing.T) {
	r := mustEngine(t, baseRules)
	mi := MediaInfo{Container: "mkv", VideoCodec: "h264", AudioCodecs: []string{"aac"}}
	got := r.DescribeMiss(mi)
	if !strings.Contains(got, "h264 -> hevc: OK") {
		t.Errorf("DescribeMiss deveria marcar a regra como OK; obteve: %s", got)
	}
}

func TestRulesAudioNotBlocking(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "áudio não casa"
    match:
      container: mkv
      video: { codec: h264 }
      audio: { codec: [aac, mp3] }
    convert:
      video: { codec: hevc }
`
	r := mustEngine(t, content)
	// áudio flac não casa com a regra, mas NÃO deve impedir a conversão
	mi := MediaInfo{Container: "matroska,webm", VideoCodec: "h264", AudioCodecs: []string{"flac"}}
	outcome, _, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("áudio divergente não deveria impedir conversão; obteve %v", outcome)
	}
	// DescribeMiss não deve apontar áudio como falha bloqueadora
	got := r.DescribeMiss(mi)
	if strings.Contains(got, "audio.codec") {
		t.Fatalf("DescribeMiss não deveria reportar áudio como falha; obteve: %s", got)
	}
	if !strings.Contains(got, "OK") {
		t.Fatalf("regra deveria estar marcada como OK (só áudio divergia); obteve: %s", got)
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
	outcome, _, ruleName, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkip {
		t.Fatalf("esperava skip, obteve %v", outcome)
	}
	if ruleName != "av1 skip" {
		t.Fatalf("esperava ruleName 'av1 skip', obteve %q", ruleName)
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
	outcome, _, _, err := r.Evaluate(mi)
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
	outcome, spec, _, err := r.Evaluate(MediaInfo{Container: "any", VideoCodec: "h264"})
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

func TestRulesContainerNormalization(t *testing.T) {
	// regra "h264 -> hevc" aceita container mkv; mídia real reporta matroska,webm
	r := mustEngine(t, baseRules)
	mi := MediaInfo{Container: "matroska,webm", VideoCodec: "h264", AudioCodecs: []string{"aac"}}
	outcome, _, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("matroska deveria casar container mkv; obteve outcome %v", outcome)
	}
}

func TestRulesContainerMp4FormatName(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs, defaults: { video: { codec: hevc, crf: 22, preset: slow }, audio: { codec: copy } } }
rules:
  - name: mp4
    match:
      container: mp4
      video: { codec: h264 }
    convert:
      video: { codec: hevc }
`
	r := mustEngine(t, content)
	// ffprobe de um MP4 real retorna "mov,mp4,m4a,3gp,3g2,mj2"
	mi := MediaInfo{Container: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodec: "h264"}
	outcome, _, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("format_name de mp4 deveria casar container mp4; obteve %v", outcome)
	}
}

func TestRulesLossless(t *testing.T) {
	content := `
version: 1
global:
  staging_dir: /tmp/xs
  defaults:
    video: { codec: hevc, crf: 22, preset: slow }
rules:
  - name: lossless_rule
    match:
      video: { codec: h264 }
    convert:
      video: { codec: hevc, lossless: true }
`
	r := mustEngine(t, content)
	mi := MediaInfo{Container: "mkv", VideoCodec: "h264"}
	outcome, spec, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("esperava convert, obteve %v", outcome)
	}
	if !spec.VideoLossless {
		t.Errorf("esperava VideoLossless = true, obteve false")
	}
	if spec.VideoCRF != 0 {
		t.Errorf("esperava VideoCRF = 0 para lossless, obteve %d (sobrescreveu com default)", spec.VideoCRF)
	}
}

// TestRulesLosslessExplicitFalseOverridesDefault reproduz o bug em que uma
// regra com `lossless: false` explícito (para usar CRF/lossy) era
// silenciosamente sobrescrita pelo default global `lossless: true`, pois
// ambos os casos ("não especificado" e "explicitamente false") produziam o
// mesmo bool zero-value. Com Lossless como *bool, a escolha explícita da
// regra deve prevalecer sobre o default.
func TestRulesLosslessExplicitFalseOverridesDefault(t *testing.T) {
	content := `
version: 1
global:
  staging_dir: /tmp/xs
  defaults:
    video: { codec: hevc, crf: 22, preset: slow, lossless: true }
rules:
  - name: opt_out_of_lossless
    match:
      video: { codec: h264 }
    convert:
      video: { codec: hevc, lossless: false, crf: 28 }
`
	r := mustEngine(t, content)
	mi := MediaInfo{Container: "mkv", VideoCodec: "h264"}
	outcome, spec, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("esperava convert, obteve %v", outcome)
	}
	if spec.VideoLossless {
		t.Errorf("esperava VideoLossless = false (regra opta explicitamente por lossy), obteve true (default global sobrescreveu)")
	}
	if spec.VideoCRF != 28 {
		t.Errorf("esperava VideoCRF = 28 (definido pela regra), obteve %d", spec.VideoCRF)
	}
}

// TestMergeSpecAutoApproveRuleOverridesGlobal cobre a resolução de
// TargetSpec.AutoApprove: override por regra vence o default global; quando
// nem regra nem default especificam nada, o resultado é false (exige
// aprovação manual — comportamento novo a partir da v1.2).
func TestMergeSpecAutoApproveRuleOverridesGlobal(t *testing.T) {
	content := `
version: 1
global:
  staging_dir: /tmp/xs
  defaults:
    video: { codec: hevc }
    auto_approve: true
rules:
  - name: regra_sem_override
    match:
      container: mkv
    convert:
      video: { codec: hevc }
  - name: regra_com_override
    match:
      container: mp4
    convert:
      video: { codec: hevc }
    auto_approve: false
`
	r := mustEngine(t, content)

	// Sem override na regra: herda o default global (true).
	_, spec1, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264"})
	if err != nil {
		t.Fatal(err)
	}
	if !spec1.AutoApprove {
		t.Errorf("esperava AutoApprove=true herdado do default global, obteve false")
	}

	// Com override explícito na regra: vence o default global.
	_, spec2, _, err := r.Evaluate(MediaInfo{Container: "mp4", VideoCodec: "h264"})
	if err != nil {
		t.Fatal(err)
	}
	if spec2.AutoApprove {
		t.Errorf("esperava AutoApprove=false (override da regra), obteve true (default global vazou)")
	}
}

// TestMergeSpecAutoApproveDefaultsFalse cobre o caso central da v1.2: sem
// auto_approve configurado em lugar nenhum (nem regra, nem default global), o
// resultado deve ser false — exige aprovação manual por padrão.
func TestMergeSpecAutoApproveDefaultsFalse(t *testing.T) {
	r := mustEngine(t, baseRules) // baseRules não define auto_approve em lugar nenhum
	_, spec, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", AudioCodecs: []string{"aac"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.AutoApprove {
		t.Errorf("esperava AutoApprove=false por padrão (nada configurado), obteve true")
	}
}

// TestRulesMatchMinMaxHeight cobre o match numérico por altura de vídeo
// (video.min_height/max_height), isolado.
func TestRulesMatchMinMaxHeight(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "downscale 4k"
    match:
      video: { min_height: 1440 }
    convert:
      video: { codec: hevc, max_height: 1080 }
`
	r := mustEngine(t, content)

	// Altura acima do min_height: casa.
	outcome, spec, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", Height: 2160})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("altura 2160 >= min_height 1440 deveria casar; obteve %v", outcome)
	}
	if spec.VideoMaxHeight != 1080 {
		t.Errorf("esperava VideoMaxHeight=1080 propagado de convert.video.max_height; obteve %d", spec.VideoMaxHeight)
	}

	// Altura abaixo do min_height: não casa.
	outcome, _, _, err = r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", Height: 720})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("altura 720 < min_height 1440 não deveria casar; obteve %v", outcome)
	}
}

// TestRulesMatchMaxHeightOnly cobre o match por video.max_height isolado
// (limite superior sem piso).
func TestRulesMatchMaxHeightOnly(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "só conteúdo <= 720p"
    match:
      video: { max_height: 720 }
    convert:
      video: { codec: hevc }
`
	r := mustEngine(t, content)

	outcome, _, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", Height: 480})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("altura 480 <= max_height 720 deveria casar; obteve %v", outcome)
	}

	outcome, _, _, err = r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("altura 1080 > max_height 720 não deveria casar; obteve %v", outcome)
	}
}

// TestRulesMatchHeightCombinedWithCodec garante que o critério de altura se
// combina (AND) com o critério de codec já existente, não substitui.
func TestRulesMatchHeightCombinedWithCodec(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "h264 4k -> hevc downscale"
    match:
      video: { codec: h264, min_height: 1440 }
    convert:
      video: { codec: hevc, max_height: 1080 }
`
	r := mustEngine(t, content)

	// Codec casa, altura casa: convert.
	outcome, _, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", Height: 2160})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("codec+altura deveriam casar; obteve %v", outcome)
	}

	// Codec casa, altura NÃO casa: não deve casar.
	outcome, _, _, err = r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("codec casa mas altura não; não deveria casar. obteve %v", outcome)
	}

	// Altura casa, codec NÃO casa: não deve casar.
	outcome, _, _, err = r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "hevc", Height: 2160})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("altura casa mas codec não; não deveria casar. obteve %v", outcome)
	}
}

// TestRulesMatchMinBitrateKbps cobre o match numérico por bitrate mínimo de
// vídeo (video.min_bitrate_kbps), comparando contra MediaInfo.VideoBitrate
// (em bps, convertido para kbps).
func TestRulesMatchMinBitrateKbps(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "só bitrate alto"
    match:
      video: { min_bitrate_kbps: 8000 }
    convert:
      video: { codec: hevc }
`
	r := mustEngine(t, content)

	// 10000 kbps (10_000_000 bps) >= 8000 kbps: casa.
	outcome, _, _, err := r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", VideoBitrate: 10_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("bitrate 10000kbps >= min 8000kbps deveria casar; obteve %v", outcome)
	}

	// 4000 kbps < 8000 kbps: não casa.
	outcome, _, _, err = r.Evaluate(MediaInfo{Container: "mkv", VideoCodec: "h264", VideoBitrate: 4_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeSkipNoRule {
		t.Fatalf("bitrate 4000kbps < min 8000kbps não deveria casar; obteve %v", outcome)
	}
}

func TestRulesHWAccel(t *testing.T) {
	content := `
version: 1
global:
  staging_dir: /tmp/xs
  defaults:
    video: { codec: hevc, crf: 22, preset: slow, hwaccel: vaapi }
rules:
  - name: rule_without_hwaccel
    match:
      container: mkv
      video: { codec: h264 }
    convert:
      video: { codec: hevc }
  - name: rule_with_specific_hwaccel
    match:
      container: mp4
      video: { codec: h264 }
    convert:
      video: { codec: hevc, hwaccel: nvenc }
`
	r := mustEngine(t, content)

	// Caso 1: Regra sem hwaccel específico deve herdar o global default (vaapi)
	mi1 := MediaInfo{Container: "mkv", VideoCodec: "h264"}
	_, spec1, _, err := r.Evaluate(mi1)
	if err != nil {
		t.Fatal(err)
	}
	if spec1.VideoHWAccel != "vaapi" {
		t.Errorf("esperava hwaccel herdado 'vaapi', obteve %q", spec1.VideoHWAccel)
	}

	// Caso 2: Regra com hwaccel específico deve vencer o default (nvenc)
	mi2 := MediaInfo{Container: "mp4", VideoCodec: "h264"}
	_, spec2, _, err := r.Evaluate(mi2)
	if err != nil {
		t.Fatal(err)
	}
	if spec2.VideoHWAccel != "nvenc" {
		t.Errorf("esperava hwaccel específico 'nvenc', obteve %q", spec2.VideoHWAccel)
	}
}

// TestItemMarshalJSONRoundTrip cobre o item 1 da tabela de mudanças do core:
// um Item com exatamente um valor deve round-tripar como escalar JSON (ex.:
// "h264"), NÃO como array de um elemento ({"Values":["h264"]} é o shape
// quebrado que existia antes do MarshalJSON); um Item com múltiplos valores
// deve round-tripar como array; um Item vazio (coringa) round-tripa como
// string vazia e continua vazio depois de desserializado de volta.
func TestItemMarshalJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		item       Item
		wantJSON   string
		wantValues []string
	}{
		{"escalar único", Item{Values: []string{"h264"}}, `"h264"`, []string{"h264"}},
		{"lista com múltiplos", Item{Values: []string{"h264", "hevc"}}, `["h264","hevc"]`, []string{"h264", "hevc"}},
		{"vazio (coringa)", Item{}, `""`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.item)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(data) != tc.wantJSON {
				t.Fatalf("Marshal(%+v) = %s, want %s (escalar único não deve virar array de 1 elemento)", tc.item, data, tc.wantJSON)
			}
			var back Item
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(back.Values, tc.wantValues) {
				t.Errorf("round-trip Values = %#v, want %#v", back.Values, tc.wantValues)
			}
		})
	}
}

// TestItemMarshalYAMLRoundTrip espelha TestItemMarshalJSONRoundTrip para o
// encoder YAML (gopkg.in/yaml.v3) — mesmo contrato: escalar único → escalar,
// não sequência de 1 elemento.
func TestItemMarshalYAMLRoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		item       Item
		wantValues []string
	}{
		{"escalar único", Item{Values: []string{"h264"}}, []string{"h264"}},
		{"lista com múltiplos", Item{Values: []string{"h264", "hevc"}}, []string{"h264", "hevc"}},
		{"vazio (coringa)", Item{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := yaml.Marshal(tc.item)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if len(tc.item.Values) == 1 {
				// Escalar único deve serializar como escalar YAML puro (uma
				// linha, sem marcador de sequência "- "), não como lista de 1.
				if strings.Contains(string(data), "- ") {
					t.Fatalf("Marshal(%+v) = %q, não deveria conter marcador de sequência YAML", tc.item, data)
				}
			}
			var back Item
			if err := yaml.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(back.Values, tc.wantValues) {
				t.Errorf("round-trip Values = %#v, want %#v", back.Values, tc.wantValues)
			}
		})
	}
}

// TestRuleEnabledSkipped cobre o item 2 da tabela de mudanças do core: uma
// regra com `enabled: false` explícito nunca casa em Evaluate (mesmo que
// seus critérios de match combinem com a mídia) nem aparece como candidata
// em DescribeMiss — nem como falha, nem como "OK". Uma regra SEM o campo
// (nil) continua habilitada por padrão (retrocompat).
func TestRuleEnabledSkipped(t *testing.T) {
	content := `
version: 1
global: { staging_dir: /tmp/xs }
rules:
  - name: "desabilitada"
    enabled: false
    match:
      video: { codec: h264 }
    convert:
      video: { codec: av1 }
  - name: "habilitada (sem campo, retrocompat)"
    match:
      video: { codec: h264 }
    convert:
      video: { codec: hevc }
`
	r := mustEngine(t, content)
	mi := MediaInfo{Container: "mkv", VideoCodec: "h264"}

	outcome, spec, _, err := r.Evaluate(mi)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConvert {
		t.Fatalf("esperava convert (via segunda regra, habilitada); obteve %v", outcome)
	}
	if spec.VideoCodec != "hevc" {
		t.Fatalf("regra desabilitada não deveria ter sido escolhida; esperava hevc (2ª regra), obteve %q", spec.VideoCodec)
	}

	miss := r.DescribeMiss(mi)
	if strings.Contains(miss, "desabilitada") {
		t.Errorf("DescribeMiss não deveria mencionar a regra desabilitada; obteve: %s", miss)
	}
	if !strings.Contains(miss, "habilitada (sem campo, retrocompat): OK") {
		t.Errorf("DescribeMiss deveria marcar a regra habilitada como OK; obteve: %s", miss)
	}
}

// TestRuleFileValidate cobre o item 3 da tabela de mudanças do core: casos
// válidos e cada uma das condições de invalidez descritas na proposta
// (regras vazias, action inválida, nome vazio, min_saving_pct fora de
// [0,100], default_driver vazio).
func TestRuleFileValidate(t *testing.T) {
	validBase := func() RuleFile {
		return RuleFile{
			Version: 1,
			Global: RuleFileGlobal{
				DefaultDriver: "ffmpeg",
				SpaceSaving:   SpaceSavingPolicy{MinSavingPct: 15},
			},
			Rules: []Rule{
				{Name: "r1", Action: ActionConvert, Convert: ConvertSpec{Video: &TargetSpecVideo{Codec: "hevc"}}},
			},
		}
	}

	t.Run("válido", func(t *testing.T) {
		if err := validBase().Validate(); err != nil {
			t.Errorf("esperava válido, obteve erro: %v", err)
		}
	})

	t.Run("válido com action skip e min_saving_pct nos extremos", func(t *testing.T) {
		f := validBase()
		f.Rules[0].Action = ActionSkip
		f.Global.SpaceSaving.MinSavingPct = 0
		if err := f.Validate(); err != nil {
			t.Errorf("esperava válido (0 é extremo válido de [0,100]), obteve erro: %v", err)
		}
		f.Global.SpaceSaving.MinSavingPct = 100
		if err := f.Validate(); err != nil {
			t.Errorf("esperava válido (100 é extremo válido de [0,100]), obteve erro: %v", err)
		}
	})

	t.Run("rules vazio", func(t *testing.T) {
		f := validBase()
		f.Rules = nil
		if err := f.Validate(); err == nil {
			t.Error("esperava erro para rules vazio")
		}
	})

	t.Run("nome de regra vazio", func(t *testing.T) {
		f := validBase()
		f.Rules[0].Name = "   "
		if err := f.Validate(); err == nil {
			t.Error("esperava erro para nome de regra vazio")
		}
	})

	t.Run("action inválida", func(t *testing.T) {
		f := validBase()
		f.Rules[0].Action = "transmogrify"
		if err := f.Validate(); err == nil {
			t.Error("esperava erro para action inválida")
		}
	})

	t.Run("min_saving_pct negativo", func(t *testing.T) {
		f := validBase()
		f.Global.SpaceSaving.MinSavingPct = -1
		if err := f.Validate(); err == nil {
			t.Error("esperava erro para min_saving_pct negativo")
		}
	})

	t.Run("min_saving_pct acima de 100", func(t *testing.T) {
		f := validBase()
		f.Global.SpaceSaving.MinSavingPct = 101
		if err := f.Validate(); err == nil {
			t.Error("esperava erro para min_saving_pct > 100")
		}
	})

	t.Run("default_driver vazio", func(t *testing.T) {
		f := validBase()
		f.Global.DefaultDriver = ""
		if err := f.Validate(); err == nil {
			t.Error("esperava erro para default_driver vazio")
		}
	})
}

func TestRulesVideoTune(t *testing.T) {
	content := `
version: 1
global:
  staging_dir: /tmp/xs
  defaults:
    video: { codec: hevc, crf: 22, preset: slow, tune: film }
rules:
  - name: anime_tune_rule
    match:
      video: { codec: h264 }
    convert:
      video: { codec: libx264, tune: animation }
  - name: default_tune_rule
    match:
      video: { codec: mpeg2video }
    convert:
      video: { codec: hevc }
`
	r := mustEngine(t, content)

	// Regra 1: sobrescreve tune para "animation"
	mi1 := MediaInfo{VideoCodec: "h264"}
	outcome1, spec1, _, err := r.Evaluate(mi1)
	if err != nil {
		t.Fatal(err)
	}
	if outcome1 != OutcomeConvert {
		t.Fatalf("esperava OutcomeConvert, obteve %v", outcome1)
	}
	if spec1.VideoTune != "animation" {
		t.Fatalf("esperava VideoTune=animation, obteve %q", spec1.VideoTune)
	}

	// Regra 2: herda tune "film" dos defaults globais
	mi2 := MediaInfo{VideoCodec: "mpeg2video"}
	outcome2, spec2, _, err := r.Evaluate(mi2)
	if err != nil {
		t.Fatal(err)
	}
	if outcome2 != OutcomeConvert {
		t.Fatalf("esperava OutcomeConvert, obteve %v", outcome2)
	}
	if spec2.VideoTune != "film" {
		t.Fatalf("esperava VideoTune=film herdado do default global, obteve %q", spec2.VideoTune)
	}
}

