package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SpaceSavingPolicy define a política de eficiência de espaço (RF05, seção 7).
type SpaceSavingPolicy struct {
	MinSavingPct   float64 `yaml:"min_saving_pct" json:"min_saving_pct"`
	FallbackAction string  `yaml:"fallback_action" json:"fallback_action"` // rollback
}

// RuleDefaults são os alvos default aplicados quando a regra não especifica.
type RuleDefaults struct {
	Video struct {
		Codec    string `yaml:"codec" json:"codec"`
		CRF      int    `yaml:"crf" json:"crf"`
		Preset   string `yaml:"preset" json:"preset"`
		Lossless bool   `yaml:"lossless" json:"lossless"`
	} `yaml:"video" json:"video"`
	Audio struct {
		Codec   string `yaml:"codec" json:"codec"`
		Bitrate string `yaml:"bitrate" json:"bitrate"`
	} `yaml:"audio" json:"audio"`
	Container string `yaml:"container" json:"container"`
}

// ItemsMatch permite que um campo case por igualdade escalar OU por lista OR.
type ItemsMatch struct {
	Values []string
}

// MatchVideo representa os critérios de correspondência de vídeo.
type MatchVideo struct {
	Codec Item `yaml:"codec" json:"codec"`
}

// MatchAudio representa os critérios de correspondência de áudio.
type MatchAudio struct {
	Codec Item `yaml:"codec" json:"codec"` // um ou vários
}

// Match agrupa todos os critérios de uma regra.
type Match struct {
	Container Item       `yaml:"container" json:"container"`
	Video     MatchVideo `yaml:"video" json:"video"`
	Audio     MatchAudio `yaml:"audio" json:"audio"`
}

// Action enum para ações da regra.
type RuleAction string

const (
	ActionConvert RuleAction = "convert"
	ActionSkip    RuleAction = "skip"
)

// ConvertSpec sobrepõe quebra de uma regra (sparse → merge com defaults).
type ConvertSpec struct {
	Video     *TargetSpecVideo `yaml:"video" json:"video"`
	Audio     *TargetSpecAudio `yaml:"audio" json:"audio"`
	Container string           `yaml:"container" json:"container"`
}

// TargetSpecVideo carrega campos de vídeo.
type TargetSpecVideo struct {
	Codec    string `yaml:"codec" json:"codec"`
	CRF      int    `yaml:"crf" json:"crf"`
	Preset   string `yaml:"preset" json:"preset"`
	Lossless bool   `yaml:"lossless" json:"lossless"`
}

// TargetSpecAudio carrega campos de áudio.
type TargetSpecAudio struct {
	Codec   string `yaml:"codec" json:"codec"`
	Bitrate string `yaml:"bitrate" json:"bitrate"`
}

// Rule representa uma regra declarativa (RF03).
type Rule struct {
	Name    string      `yaml:"name" json:"name"`
	Enabled *bool       `yaml:"enabled" json:"enabled"`
	Match   Match       `yaml:"match" json:"match"`
	Action  RuleAction  `yaml:"action" json:"action"`
	Convert ConvertSpec `yaml:"convert" json:"convert"`
}

// RuleFile é a estrutura raiz do arquivo de regras.
type RuleFile struct {
	Version int            `yaml:"version" json:"version"`
	Global  RuleFileGlobal `yaml:"global" json:"global"`
	Rules   []Rule         `yaml:"rules" json:"rules"`
	Ignore  IgnoreRules    `yaml:"ignore" json:"ignore"`
}

// RuleFileGlobal carrega configuração global.
type RuleFileGlobal struct {
	DefaultDriver string            `yaml:"default_driver" json:"default_driver"`
	StagingDir    string            `yaml:"staging_dir" json:"staging_dir"`
	SpaceSaving   SpaceSavingPolicy `yaml:"space_saving" json:"space_saving"`
	Defaults      RuleDefaults      `yaml:"defaults" json:"defaults"`
}

// IgnoreRules define arquivos/dirs que jamais serão processados.
type IgnoreRules struct {
	DirContains  []string `yaml:"dir_contains" json:"dir_contains"`
	FileSuffix   []string `yaml:"file_suffix" json:"file_suffix"`
	MinSizeBytes int64    `yaml:"min_size_bytes" json:"min_size_bytes"`
}

// Item representa um match que aceita valor escalar ou lista.
// Para simplificar o primeiro-match, aqui guardamos valores escalares comparados.
type Item struct {
	Values []string
}

// UnmarshalYAML aceita "value" ou ["a","b"].
func (i *Item) UnmarshalYAML(node *yaml.Node) error {
	return i.fromNode(node)
}

// UnmarshalJSON aceita "value" ou ["a","b"].
func (i *Item) UnmarshalJSON(data []byte) error {
	var scalar string
	if err := json.Unmarshal(data, &scalar); err == nil {
		if scalar != "" {
			i.Values = []string{strings.ToLower(scalar)}
		}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, v := range list {
		v = strings.ToLower(v)
		if v != "" {
			i.Values = append(i.Values, v)
		}
	}
	return nil
}

func (i *Item) fromNode(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return nil
		}
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		if s != "" {
			i.Values = []string{strings.ToLower(s)}
		}
	case yaml.SequenceNode:
		for _, n := range node.Content {
			var s string
			if err := n.Decode(&s); err != nil {
				return err
			}
			s = strings.ToLower(s)
			if s != "" {
				i.Values = append(i.Values, s)
			}
		}
	default:
		return fmt.Errorf("item de match inválido")
	}
	return nil
}

// matches verifica se o valor (lowercased) está na lista de critérios.
func (i Item) matches(value string) bool {
	if len(i.Values) == 0 {
		return true // criterio ausente = coringa (casa com qualquer)
	}
	value = strings.ToLower(value)
	for _, v := range i.Values {
		if v == value {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool { return &b }

// RulesEngine carrega e avalia as regras declarativas.
type RulesEngine struct {
	file RuleFile
}

// NewRulesEngine lê um arquivo YAML ou JSON de regras.
func NewRulesEngine(path string) (*RulesEngine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ler rules: %w", err)
	}
	var f RuleFile
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(data, &f)
	default:
		err = yaml.Unmarshal(data, &f)
	}
	if err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	if len(f.Rules) == 0 {
		return nil, errors.New("rules não contém regras")
	}
	if f.Global.SpaceSaving.MinSavingPct <= 0 {
		f.Global.SpaceSaving.MinSavingPct = 15
	}
	if f.Global.SpaceSaving.FallbackAction == "" {
		f.Global.SpaceSaving.FallbackAction = "rollback"
	}
	if f.Global.DefaultDriver == "" {
		f.Global.DefaultDriver = "ffmpeg"
	}
	if f.Global.StagingDir == "" {
		f.Global.StagingDir = ".codecany_tmp"
	}
	return &RulesEngine{file: f}, nil
}

// ShouldIgnore verifica se um caminho está na lista de ignores.
func (r *RulesEngine) ShouldIgnore(path string, size int64) bool {
	ig := r.file.Ignore
	if size > 0 && ig.MinSizeBytes > 0 && size < ig.MinSizeBytes {
		return true
	}
	for _, suf := range ig.FileSuffix {
		if strings.HasSuffix(strings.ToLower(path), strings.ToLower(suf)) {
			return true
		}
	}
	dir := strings.ToLower(filepath.Dir(path))
	for _, frag := range ig.DirContains {
		if strings.Contains(dir, strings.ToLower(frag)) {
			return true
		}
	}
	return false
}

// Global retorna as configurações globais.
func (r *RulesEngine) Global() RuleFileGlobal { return r.file.Global }

// Global-RulesEngine avaliam o resultado da avaliação de uma mídia.
type RuleOutcome int

const (
	OutcomeConvert RuleOutcome = iota
	OutcomeSkip
	OutcomeSkipNoRule
)

// Evaluate aplica first-match wins sobre uma MediaInfo (seção 4.2).
// Retorna o TargetSpec (merged com defaults) e o outcome.
func (r *RulesEngine) Evaluate(mi MediaInfo) (RuleOutcome, TargetSpec, error) {
	for _, rule := range r.file.Rules {
		if !ruleMatches(rule, mi) {
			continue
		}
		if rule.Action == ActionSkip {
			return OutcomeSkip, TargetSpec{}, nil
		}
		return OutcomeConvert, mergeSpec(rule.Convert, r.file.Global.Defaults), nil
	}
	return OutcomeSkipNoRule, TargetSpec{}, nil
}

func ruleMatches(rule Rule, mi MediaInfo) bool {
	if !containerMatches(rule.Match.Container, mi.Container) {
		return false
	}
	if !rule.Match.Video.Codec.matches(mi.VideoCodec) {
		return false
	}
	// O áudio NÃO é impeditivo: critérios de áudio podem existir nas regras
	// como intenção, mas um codec de áudio que não casa não impede a conversão
	// do vídeo. O `convert.audio` determina a saída (habitualmente `copy`, sem
	// perda geracional).
	return true
}

// containerAliases NORMALiza nomes de container: regras costumam usar extensões
// amigáveis ("mkv", "mp4"), enquanto o ffprobe reporta o format_name real
// ("matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2"). O mapeamento abaixo une as duas
// visões para que "container: mkv" corresponda a arquivos matroska reais.
var containerAliases = map[string][]string{
	"mkv":           {"mkv", "matroska"},
	"matroska":      {"mkv", "matroska"},
	"webm":          {"webm"},
	"mp4":           {"mp4", "mov"},
	"mov":           {"mp4", "mov"},
	"m4v":           {"mp4"},
	"m4a":           {"mp4"},
	"avi":           {"avi"},
	"mpegts":        {"mpegts", "ts"},
	"ts":            {"mpegts", "ts"},
	"m2ts":          {"m2ts"},
	"flv":           {"flv"},
	"ogg":           {"ogg"},
	"wmv":           {"wmv"},
	"auto":          {}, // coringa
}

// containerMatches compara o container reportado pelo prober contra os valores
// da regra, normalizando format_name do ffprobe (lista separada por vírgula)
// e aliases (ex.: matroska ↔ mkv). Critério vazio é coringa (casa com tudo).
func containerMatches(crit Item, container string) bool {
	if len(crit.Values) == 0 {
		return true
	}
	for _, c := range containerSet(normalizeContainer(container)) {
		for _, want := range crit.Values {
			want = strings.ToLower(strings.TrimSpace(want))
			if want == "" {
				continue
			}
			for _, alias := range containerAliases[want] {
				if alias == c {
					return true
				}
			}
		}
	}
	return false
}

// normalizeContainer converte o format_name (ou extensão) numa lista de tokens.
func normalizeContainer(v string) []string {
	return strings.Split(v, ",")
}

// containerSet expande cada token pela tabela de aliases num conjunto canônico.
func containerSet(tokens []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, t := range tokens {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if al, ok := containerAliases[t]; ok && len(al) > 0 {
			for _, a := range al {
				add(a)
			}
		} else {
			add(t)
		}
	}
	return out
}

// DescribeMiss gera um diagnóstico de por que cada regra não casou com a mídia.
// Para cada regra são apontados os critérios (container/video) que falharam,
// mostrando o valor real da mídia vs. os valores aceitos pela regra. O áudio
// não é reportado como falha, pois não é impeditivo para a conversão.
func (r *RulesEngine) DescribeMiss(mi MediaInfo) string {
	var parts []string
	for _, rule := range r.file.Rules {
		var fails []string
		if crit, ok := containerMismatch(rule.Match.Container, mi.Container); !ok {
			fails = append(fails, "container("+crit+")")
		}
		if crit, ok := itemMismatch(rule.Match.Video.Codec, mi.VideoCodec); !ok {
			fails = append(fails, "video.codec("+crit+")")
		}
		switch {
		case len(fails) == 0:
			parts = append(parts, rule.Name+": OK")
		default:
			parts = append(parts, fmt.Sprintf("%s: %s", rule.Name, strings.Join(fails, "; ")))
		}
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, "; "))
}

// itemMismatch relata critério não satisfeito; (reason, false) se falhou.
func itemMismatch(crit Item, actual string) (string, bool) {
	if len(crit.Values) == 0 {
		return "", true // coringa
	}
	actual = strings.ToLower(actual)
	for _, v := range crit.Values {
		if v == actual {
			return "", true
		}
	}
	return fmt.Sprintf("media=%s, esperado=%v", actual, crit.Values), false
}

// containerMismatch relata por que o container da mídia não casou com a regra.
func containerMismatch(crit Item, container string) (string, bool) {
	if len(crit.Values) == 0 {
		return "", true // coringa
	}
	if containerMatches(crit, container) {
		return "", true
	}
	return fmt.Sprintf("media=%v, esperado=%v", container, crit.Values), false
}

func mergeSpec(c ConvertSpec, def RuleDefaults) TargetSpec {
	out := TargetSpec{
		Container: def.Container,
	}
	if c.Video != nil {
		out.VideoCodec = c.Video.Codec
		out.VideoCRF = c.Video.CRF
		out.VideoPreset = c.Video.Preset
		out.VideoLossless = c.Video.Lossless
	}
	if out.VideoCodec == "" {
		out.VideoCodec = def.Video.Codec
	}
	if !out.VideoLossless {
		out.VideoLossless = def.Video.Lossless
	}
	if out.VideoCRF == 0 && !out.VideoLossless {
		out.VideoCRF = def.Video.CRF
	}
	if out.VideoPreset == "" {
		out.VideoPreset = def.Video.Preset
	}
	if c.Audio != nil {
		out.AudioCodec = c.Audio.Codec
		out.AudioBitrate = c.Audio.Bitrate
	}
	if out.AudioCodec == "" {
		out.AudioCodec = def.Audio.Codec
	}
	if out.AudioBitrate == "" {
		out.AudioBitrate = def.Audio.Bitrate
	}
	if c.Container != "" {
		out.Container = c.Container
	}
	return out
}
