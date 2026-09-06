package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/FM0Ura/codecany/pkg/core"
)

// rulesTestRequest é o corpo de POST /api/rules/test: OU um path de arquivo
// a sondar via a.Prober, OU um core.MediaInfo já pronto (útil pra testar uma
// regra sem ter um arquivo real disponível). Se ambos vierem, path vence.
type rulesTestRequest struct {
	Path      string          `json:"path,omitempty"`
	MediaInfo *core.MediaInfo `json:"media_info,omitempty"`
}

// rulesTestResponse é o shape de resposta de POST /api/rules/test.
type rulesTestResponse struct {
	Outcome      string          `json:"outcome"`
	TargetSpec   core.TargetSpec `json:"target_spec"`
	MatchedRule  string          `json:"matched_rule,omitempty"`
	DescribeMiss string          `json:"describe_miss"`
}

// handleGetRules atende GET /api/rules com o RuleFile atualmente em uso
// (item 13 — RulesEngine.File()). Reflete o snapshot LIVE: Rules/Global.
// Defaults são sempre os mais recentes aplicados via Reload; os demais
// campos globais (staging_dir/space_saving/hwaccel_limits/notifications/
// default_driver) são os capturados no boot do servidor (ou no último PUT
// bem-sucedido, já que ReloadRules preserva-os do snapshot anterior) — ver
// RulesEngine.Reload.
func (a *App) handleGetRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.Rules.File())
}

// handlePutRules atende PUT /api/rules: recebe um core.RuleFile completo
// (o builder do frontend reenvia o array de regras inteiro, já na ordem
// desejada — reordenar é só isso, sem endpoint de reorder dedicado),
// valida via RuleFile.Validate ANTES de tocar o arquivo em disco (400 com
// mensagem clara se inválido, arquivo inalterado), serializa para YAML e
// grava com troca atômica (mesmo padrão de pkg/core/cleanup.go::
// replaceAtomic — não reexportado por pkg/core, reimplementado aqui como
// writeFileAtomic) no path de regras configurado (Config.RulesPath), e por
// fim aciona Engine.ReloadRules para o hot-reload do casamento de regras
// entrar em vigor sem reiniciar o processo.
func (a *App) handlePutRules(w http.ResponseWriter, r *http.Request) {
	var f core.RuleFile
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeJSONError(w, http.StatusBadRequest, "corpo inválido: "+err.Error())
		return
	}
	if err := f.Validate(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := yaml.Marshal(&f)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "serializar regras: "+err.Error())
		return
	}
	if err := writeFileAtomic(a.Config.RulesPath, data); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "gravar regras: "+err.Error())
		return
	}
	if err := a.Engine.ReloadRules(a.Config.RulesPath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "regras gravadas, mas falha ao recarregar: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.Rules.File())
}

// handleTestRule atende POST /api/rules/test: avalia as regras ATUALMENTE
// em uso (a.Rules, o mesmo snapshot live consultado por handleGetRules)
// contra uma MediaInfo — obtida por sondagem (`path`) ou fornecida direto
// (`media_info`) — retornando o outcome, o TargetSpec resultante e o
// diagnóstico verboso de DescribeMiss.
func (a *App) handleTestRule(w http.ResponseWriter, r *http.Request) {
	var body rulesTestRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "corpo inválido: "+err.Error())
		return
	}

	var mi core.MediaInfo
	switch {
	case body.Path != "":
		if a.Prober == nil {
			writeJSONError(w, http.StatusInternalServerError, "prober não disponível")
			return
		}
		probed, err := a.Prober.Probe(body.Path)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "probe falhou: "+err.Error())
			return
		}
		mi = probed
	case body.MediaInfo != nil:
		mi = *body.MediaInfo
	default:
		writeJSONError(w, http.StatusBadRequest, "informe path ou media_info")
		return
	}

	outcome, target, ruleName, err := a.Rules.Evaluate(mi)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rulesTestResponse{
		Outcome:      ruleOutcomeString(outcome),
		TargetSpec:   target,
		MatchedRule:  ruleName,
		DescribeMiss: a.Rules.DescribeMiss(mi),
	})
}

// ruleOutcomeString converte core.RuleOutcome (enum inteiro interno de
// pkg/core) num rótulo estável para a API JSON.
func ruleOutcomeString(o core.RuleOutcome) string {
	switch o {
	case core.OutcomeConvert:
		return "convert"
	case core.OutcomeSkip:
		return "skip"
	default:
		return "skip_no_rule"
	}
}

// writeFileAtomic grava data em path de forma atômica: escreve num arquivo
// temporário no MESMO diretório de path (garantindo o mesmo device, sem
// necessidade do fallback de cópia cross-device que
// pkg/core/cleanup.go::replaceAtomic implementa para staging→original, que
// podem viver em filesystems diferentes) e troca via os.Rename. Mesmo
// padrão de troca atômica já usado pelo core, reimplementado aqui porque
// replaceAtomic não é exportado por pkg/core.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criar diretório: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".rules-*.tmp")
	if err != nil {
		return fmt.Errorf("criar temporário: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("escrever temporário: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("fechar temporário: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renomear atomicamente: %w", err)
	}
	return nil
}
