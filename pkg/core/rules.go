package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

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
		Lossless *bool  `yaml:"lossless" json:"lossless"`
		HWAccel  string `yaml:"hwaccel" json:"hwaccel"`
	} `yaml:"video" json:"video"`
	Audio struct {
		Codec   string `yaml:"codec" json:"codec"`
		Bitrate string `yaml:"bitrate" json:"bitrate"`
	} `yaml:"audio" json:"audio"`
	Container string `yaml:"container" json:"container"`

	// AutoApprove é o default global de aprovação automática (ver Rule.AutoApprove
	// para o override por regra). *bool para distinguir "não configurado em
	// lugar nenhum" (nil → boolVal(nil)==false, exige aprovação manual) de
	// "explicitamente false" — mesmo padrão já usado para Video.Lossless.
	AutoApprove *bool `yaml:"auto_approve" json:"auto_approve"`
}

// ItemsMatch permite que um campo case por igualdade escalar OU por lista OR.
type ItemsMatch struct {
	Values []string
}

// MatchVideo representa os critérios de correspondência de vídeo.
type MatchVideo struct {
	Codec Item `yaml:"codec" json:"codec"`

	// MinHeight/MaxHeight comparam numericamente (>=/<=) contra MediaInfo.Height
	// (0 = sem restrição nesse extremo). Diferente de Codec (Item), que é
	// match escalar/lista OR, estes são comparações numéricas de faixa.
	MinHeight int `yaml:"min_height" json:"min_height"`
	MaxHeight int `yaml:"max_height" json:"max_height"`

	// MinBitrateKbps compara numericamente (>=) contra
	// MediaInfo.VideoBitrate/1000 (0 = sem restrição).
	MinBitrateKbps int `yaml:"min_bitrate_kbps" json:"min_bitrate_kbps"`
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
// Lossless é *bool para distinguir "não especificado na regra" (nil) de
// "explicitamente false" — ver mergeSpec, que não deve deixar o default
// global sobrescrever uma escolha explícita da regra.
type TargetSpecVideo struct {
	Codec    string `yaml:"codec" json:"codec"`
	CRF      int    `yaml:"crf" json:"crf"`
	Preset   string `yaml:"preset" json:"preset"`
	Lossless *bool  `yaml:"lossless" json:"lossless"`
	HWAccel  string `yaml:"hwaccel" json:"hwaccel"`

	// MaxHeight, quando > 0, define o teto de altura do vídeo de saída (ver
	// TargetSpec.VideoMaxHeight em types.go). Só existe a nível de regra, sem
	// default global equivalente.
	MaxHeight int `yaml:"max_height" json:"max_height"`
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

	// AutoApprove, quando definido (não-nil), sobrepõe o default global
	// RuleDefaults.AutoApprove só para esta regra.
	AutoApprove *bool `yaml:"auto_approve" json:"auto_approve"`
}

// RuleFile é a estrutura raiz do arquivo de regras.
type RuleFile struct {
	Version int            `yaml:"version" json:"version"`
	Global  RuleFileGlobal `yaml:"global" json:"global"`
	Rules   []Rule         `yaml:"rules" json:"rules"`
	Ignore  IgnoreRules    `yaml:"ignore" json:"ignore"`
}

type WebhookConfig struct {
	URL    string   `yaml:"webhook_url" json:"webhook_url"`
	Events []string `yaml:"events" json:"events"`
}

// RuleFileGlobal carrega configuração global.
type RuleFileGlobal struct {
	DefaultDriver string            `yaml:"default_driver" json:"default_driver"`
	StagingDir    string            `yaml:"staging_dir" json:"staging_dir"`
	SpaceSaving   SpaceSavingPolicy `yaml:"space_saving" json:"space_saving"`
	Defaults      RuleDefaults      `yaml:"defaults" json:"defaults"`
	Notifications WebhookConfig     `yaml:"notifications" json:"notifications"`

	// HWAccelLimits limita quantas transcodificações usando um dado vendor de
	// hwaccel (ex. "nvenc", "vaapi") podem rodar simultaneamente, independente
	// do número de -workers configurado. Chave = vendor (mesmo valor literal
	// usado em convert.video.hwaccel/defaults.video.hwaccel nas regras),
	// valor = número máximo de sessões concorrentes. Vendors ausentes deste
	// mapa (ou com limite <= 0) não são limitados — seguem só o pool global
	// de workers. Ver Engine.hwSemaphores para a implementação (chan struct{}
	// como semáforo por vendor).
	HWAccelLimits map[string]int `yaml:"hwaccel_limits" json:"hwaccel_limits"`
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

// MarshalJSON serializa Item de volta a escalar OU lista, espelhando
// exatamente o shape aceito por UnmarshalJSON: zero valores → "" (coringa,
// mesmo shape de um campo "ausente" reidratado por UnmarshalJSON, que ignora
// string vazia); exatamente um valor → escalar (NÃO array de 1 elemento -
// round-tripar como escalar é o contrato esperado pela API/rules.yaml
// reescrito pela UI); mais de um valor → array. Sem este método, o
// encoding/json default serializaria sempre {"Values":[...]}, quebrando a
// API e o próprio rules.yaml editado pelo builder de regras (item 1 da
// tabela de mudanças do core).
func (i Item) MarshalJSON() ([]byte, error) {
	switch len(i.Values) {
	case 0:
		return json.Marshal("")
	case 1:
		return json.Marshal(i.Values[0])
	default:
		return json.Marshal(i.Values)
	}
}

// MarshalYAML espelha MarshalJSON para o encoder YAML (gopkg.in/yaml.v3
// reconhece a interface yaml.Marshaler via este método).
func (i Item) MarshalYAML() (interface{}, error) {
	switch len(i.Values) {
	case 0:
		return "", nil
	case 1:
		return i.Values[0], nil
	default:
		return i.Values, nil
	}
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

// boolVal lê um *bool tratando nil como false (valor "não especificado").
func boolVal(b *bool) bool { return b != nil && *b }

// Action enum válidos — usado por RuleFile.Validate.
var validRuleActions = map[RuleAction]bool{
	ActionConvert: true,
	ActionSkip:    true,
	"":            true, // vazio == ActionConvert (default histórico, ver Evaluate)
}

// Validate valida a integridade estrutural de um RuleFile antes de ser usado
// para avaliar mídias ou persistido em disco (item 3 da tabela de mudanças
// do core): regras não vazias, cada regra com nome e Action válidos,
// min_saving_pct dentro de [0,100] e default_driver não vazio. Usado tanto
// por NewRulesEngine/Reload (via loadRuleFile) quanto pelo handler
// `PUT /api/rules` do cmd/server, que precisa recusar (400) uma edição
// inválida da UI ANTES de tocar o arquivo em disco.
func (f RuleFile) Validate() error {
	if len(f.Rules) == 0 {
		return errors.New("rules não contém regras")
	}
	if strings.TrimSpace(f.Global.DefaultDriver) == "" {
		return errors.New("global.default_driver não pode ser vazio")
	}
	if f.Global.SpaceSaving.MinSavingPct < 0 || f.Global.SpaceSaving.MinSavingPct > 100 {
		return fmt.Errorf("global.space_saving.min_saving_pct deve estar entre 0 e 100 (recebido %v)", f.Global.SpaceSaving.MinSavingPct)
	}
	for i, rule := range f.Rules {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("regra #%d: nome não pode ser vazio", i)
		}
		if !validRuleActions[rule.Action] {
			return fmt.Errorf("regra %q: action inválida: %q", rule.Name, rule.Action)
		}
	}
	return nil
}

// RulesEngine carrega e avalia as regras declarativas.
//
// Thread-safety / hot-reload (item 4 da tabela de mudanças do core): o
// RuleFile completo é mantido atrás de um atomic.Pointer, trocado de uma vez
// só (CAS) a cada Reload — leituras concorrentes (Evaluate/DescribeMiss/
// ShouldIgnore/Global/File, chamadas por N workers e por qualquer handler
// HTTP) nunca observam um RuleFile parcialmente escrito, sem precisar de
// mutex/RWMutex. Reload, no entanto, NÃO substitui o snapshot inteiro: por
// design (ver Engine.ReloadRules), o hot-reload é restrito ao casamento de
// regras — só Rules e Global.Defaults do novo arquivo entram no próximo
// snapshot; todo o resto (DefaultDriver, StagingDir, SpaceSaving,
// HWAccelLimits, Notifications, Ignore, Version) é preservado do snapshot
// atual, porque o Engine já capturou esses valores (e.staging/e.integrity/
// e.hwSemaphores/e.webhook) uma única vez em NewEngine e recalculá-los em
// runtime seria perigoso (ex.: mudar staging_dir no meio de um job cujo
// Cleanup foi reconstruído deterministicamente a partir de e.staging).
type RulesEngine struct {
	snap atomic.Pointer[RuleFile]
}

// applyRuleFileDefaults preenche os valores default históricos de
// NewRulesEngine (pré-existentes a esta fase, preservados tal como estavam)
// quando o arquivo carregado não os especifica.
func applyRuleFileDefaults(f *RuleFile) {
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
}

// loadRuleFile lê e parseia (YAML ou JSON, pela extensão) um arquivo de
// regras, aplica os defaults históricos e valida o resultado via
// RuleFile.Validate — helper compartilhado por NewRulesEngine e
// RulesEngine.Reload, para que os dois caminhos de carga nunca divirjam.
func loadRuleFile(path string) (RuleFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuleFile{}, fmt.Errorf("ler rules: %w", err)
	}
	var f RuleFile
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(data, &f)
	default:
		err = yaml.Unmarshal(data, &f)
	}
	if err != nil {
		return RuleFile{}, fmt.Errorf("parse rules: %w", err)
	}
	applyRuleFileDefaults(&f)
	if err := f.Validate(); err != nil {
		return RuleFile{}, err
	}
	return f, nil
}

// NewRulesEngine lê um arquivo YAML ou JSON de regras.
func NewRulesEngine(path string) (*RulesEngine, error) {
	f, err := loadRuleFile(path)
	if err != nil {
		return nil, err
	}
	re := &RulesEngine{}
	re.snap.Store(&f)
	return re, nil
}

// Reload relê `path` e, se válido, troca atomicamente (CAS) só a parte de
// casamento de regras (Rules + Global.Defaults) do snapshot em uso — o
// restante do RuleFile (DefaultDriver, StagingDir, SpaceSaving,
// HWAccelLimits, Notifications, Ignore, Version) é preservado do snapshot
// anterior (ver comentário do struct RulesEngine). Chamado a partir de
// Engine.ReloadRules; um job em voo que já leu seu TargetSpec (ruleMatches/
// Evaluate rodam em HandleDiscovered, ANTES do job ser enfileirado) não é
// afetado por um Reload concorrente — o snapshot antigo só deixa de ser
// alcançável por NOVAS chamadas a Evaluate/DescribeMiss depois do CAS.
// Se o novo arquivo for inválido (parse ou Validate), retorna erro e o
// snapshot em uso permanece intocado.
func (r *RulesEngine) Reload(path string) error {
	next, err := loadRuleFile(path)
	if err != nil {
		return err
	}
	for {
		old := r.snap.Load()
		merged := *old
		merged.Rules = next.Rules
		merged.Global.Defaults = next.Global.Defaults
		if r.snap.CompareAndSwap(old, &merged) {
			return nil
		}
	}
}

// current retorna o snapshot ativo no momento da chamada (leitura atômica).
func (r *RulesEngine) current() RuleFile { return *r.snap.Load() }

// File retorna uma cópia do RuleFile atualmente em uso (item 13 da tabela de
// mudanças do core) — usado pelo handler `GET /api/rules` do cmd/server.
func (r *RulesEngine) File() RuleFile { return r.current() }

// ShouldIgnore verifica se um caminho está na lista de ignores.
func (r *RulesEngine) ShouldIgnore(path string, size int64) bool {
	ig := r.current().Ignore
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
func (r *RulesEngine) Global() RuleFileGlobal { return r.current().Global }

// Global-RulesEngine avaliam o resultado da avaliação de uma mídia.
type RuleOutcome int

const (
	OutcomeConvert RuleOutcome = iota
	OutcomeSkip
	OutcomeSkipNoRule
)

// Evaluate aplica first-match wins sobre uma MediaInfo (seção 4.2).
// Retorna o TargetSpec (merged com defaults) e o outcome. Usa um único
// snapshot (r.current()) para toda a avaliação — mesmo que um Reload
// concorrente troque o snapshot ativo no meio da chamada, esta avaliação vê
// um RuleFile consistente do início ao fim (nunca uma mistura das regras
// antigas com os defaults novos ou vice-versa).
func (r *RulesEngine) Evaluate(mi MediaInfo) (RuleOutcome, TargetSpec, error) {
	f := r.current()
	for _, rule := range f.Rules {
		if !ruleMatches(rule, mi) {
			continue
		}
		if rule.Action == ActionSkip {
			return OutcomeSkip, TargetSpec{}, nil
		}
		return OutcomeConvert, mergeSpec(rule, f.Global.Defaults), nil
	}
	return OutcomeSkipNoRule, TargetSpec{}, nil
}

// ruleMatches decide se `rule` casa com `mi`. Regras desabilitadas
// (Enabled != nil && !*Enabled) nunca casam — convenção INVERTIDA da usada
// por AutoApprove/Lossless (nil == habilitada, para retrocompat com
// rules.yaml existentes que não têm o campo; só `false` explícito
// desabilita). Ver item 2 da tabela de mudanças do core.
func ruleMatches(rule Rule, mi MediaInfo) bool {
	if rule.Enabled != nil && !*rule.Enabled {
		return false
	}
	if !containerMatches(rule.Match.Container, mi.Container) {
		return false
	}
	if !rule.Match.Video.Codec.matches(mi.VideoCodec) {
		return false
	}
	if !heightMatches(rule.Match.Video.MinHeight, rule.Match.Video.MaxHeight, mi.Height) {
		return false
	}
	if !minBitrateMatches(rule.Match.Video.MinBitrateKbps, mi.VideoBitrate) {
		return false
	}
	// O áudio NÃO é impeditivo: critérios de áudio podem existir nas regras
	// como intenção, mas um codec de áudio que não casa não impede a conversão
	// do vídeo. O `convert.audio` determina a saída (habitualmente `copy`, sem
	// perda geracional).
	return true
}

// heightMatches compara mi.Height (já extraído pelo prober) contra a faixa
// [minHeight, maxHeight] declarada na regra. Um extremo em 0 significa
// "sem restrição" nesse lado da faixa.
func heightMatches(minHeight, maxHeight, height int) bool {
	if minHeight > 0 && height < minHeight {
		return false
	}
	if maxHeight > 0 && height > maxHeight {
		return false
	}
	return true
}

// minBitrateMatches compara o bitrate de vídeo da mídia (em bps, convertido
// para kbps) contra o piso declarado na regra. minBitrateKbps<=0 é coringa.
func minBitrateMatches(minBitrateKbps int, videoBitrateBps int64) bool {
	if minBitrateKbps <= 0 {
		return true
	}
	return videoBitrateBps/1000 >= int64(minBitrateKbps)
}

// containerAliases NORMALiza nomes de container: regras costumam usar extensões
// amigáveis ("mkv", "mp4"), enquanto o ffprobe reporta o format_name real
// ("matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2"). O mapeamento abaixo une as duas
// visões para que "container: mkv" corresponda a arquivos matroska reais.
var containerAliases = map[string][]string{
	"mkv":      {"mkv", "matroska"},
	"matroska": {"mkv", "matroska"},
	"webm":     {"webm"},
	"mp4":      {"mp4", "mov"},
	"mov":      {"mp4", "mov"},
	"m4v":      {"mp4"},
	"m4a":      {"mp4"},
	"avi":      {"avi"},
	"mpegts":   {"mpegts", "ts"},
	"ts":       {"mpegts", "ts"},
	"m2ts":     {"m2ts"},
	"flv":      {"flv"},
	"ogg":      {"ogg"},
	"wmv":      {"wmv"},
	"auto":     {}, // coringa
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
// não é reportado como falha, pois não é impeditivo para a conversão. Regras
// desabilitadas (ver ruleMatches) são omitidas inteiramente do diagnóstico —
// não aparecem nem como falha nem como candidata "OK" (item 2).
func (r *RulesEngine) DescribeMiss(mi MediaInfo) string {
	var parts []string
	for _, rule := range r.current().Rules {
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		var fails []string
		if crit, ok := containerMismatch(rule.Match.Container, mi.Container); !ok {
			fails = append(fails, "container("+crit+")")
		}
		if crit, ok := itemMismatch(rule.Match.Video.Codec, mi.VideoCodec); !ok {
			fails = append(fails, "video.codec("+crit+")")
		}
		if crit, ok := heightMismatch(rule.Match.Video.MinHeight, rule.Match.Video.MaxHeight, mi.Height); !ok {
			fails = append(fails, "video.height("+crit+")")
		}
		if crit, ok := minBitrateMismatch(rule.Match.Video.MinBitrateKbps, mi.VideoBitrate); !ok {
			fails = append(fails, "video.bitrate("+crit+")")
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

// heightMismatch relata critério de altura não satisfeito; (reason, false) se falhou.
func heightMismatch(minHeight, maxHeight, height int) (string, bool) {
	if minHeight <= 0 && maxHeight <= 0 {
		return "", true // coringa
	}
	if heightMatches(minHeight, maxHeight, height) {
		return "", true
	}
	return fmt.Sprintf("media=%d, min=%d, max=%d", height, minHeight, maxHeight), false
}

// minBitrateMismatch relata critério de bitrate mínimo não satisfeito;
// (reason, false) se falhou.
func minBitrateMismatch(minBitrateKbps int, videoBitrateBps int64) (string, bool) {
	if minBitrateKbps <= 0 {
		return "", true // coringa
	}
	if minBitrateMatches(minBitrateKbps, videoBitrateBps) {
		return "", true
	}
	return fmt.Sprintf("media=%dkbps, esperado>=%dkbps", videoBitrateBps/1000, minBitrateKbps), false
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

func mergeSpec(rule Rule, def RuleDefaults) TargetSpec {
	c := rule.Convert
	out := TargetSpec{
		Container: def.Container,
	}
	// AutoApprove: override de regra > default global (mesmo padrão de Lossless).
	if rule.AutoApprove != nil {
		out.AutoApprove = *rule.AutoApprove
	} else {
		out.AutoApprove = boolVal(def.AutoApprove)
	}
	var losslessSetByRule bool
	if c.Video != nil {
		out.VideoCodec = c.Video.Codec
		out.VideoCRF = c.Video.CRF
		out.VideoPreset = c.Video.Preset
		out.VideoHWAccel = c.Video.HWAccel
		// Só nível de regra, sem default global (ver TargetSpec.VideoMaxHeight).
		out.VideoMaxHeight = c.Video.MaxHeight
		if c.Video.Lossless != nil {
			// A regra especificou lossless explicitamente (true ou false):
			// esse valor prevalece e não deve ser sobrescrito pelo default global.
			out.VideoLossless = *c.Video.Lossless
			losslessSetByRule = true
		}
	}
	if out.VideoCodec == "" {
		out.VideoCodec = def.Video.Codec
	}
	if out.VideoHWAccel == "" {
		out.VideoHWAccel = def.Video.HWAccel
	}
	if !losslessSetByRule {
		out.VideoLossless = boolVal(def.Video.Lossless)
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
