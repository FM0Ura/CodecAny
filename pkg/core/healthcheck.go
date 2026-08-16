package core

// HealthCheckResult é o resultado da verificação de integridade de um único
// arquivo — usado tanto pelo modo -health-check standalone da CLI
// (cmd/cli/main.go) quanto por POST /api/health-check do painel de controle
// web (cmd/server), que serializam este mesmo tipo (texto/JSON na CLI, JSON
// puro no servidor).
type HealthCheckResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Valores possíveis de HealthCheckResult.Status.
const (
	HealthStatusOK        = "ok"
	HealthStatusCorrupted = "corrupted"
)

// RunHealthCheck varre dirs via DiscoverFiles (mesmo filtro de extensão do
// watcher em tempo real) e usa files diretamente (sem filtro, mesmo
// comportamento que -file já tem no resto do programa), chamando
// verifier.Verify em cada arquivo encontrado. Nunca enfileira nada, nunca
// chama HandleDiscovered, nunca sobe Engine/Store/Watcher — só detecção, sem
// nenhuma tentativa de reparo automático (fora de escopo, ver
// docs/propostas_v1.2_adicional.md).
//
// Função pura: sem I/O de terminal nem formatação de saída — isso é
// responsabilidade do chamador (cmd/cli formata como texto ou JSON conforme
// -json; cmd/server serializa o retorno direto como JSON em
// POST /api/health-check).
//
// verifier é injetado (em vez de construído internamente via
// ffmpeg.NewVerifier()) para permitir testar esta função com um fake/mock,
// sem depender de um binário ffmpeg real.
func RunHealthCheck(dirs, files []string, verifier MediaVerifier) ([]HealthCheckResult, error) {
	found, err := DiscoverFiles(dirs)
	if err != nil {
		return nil, err
	}
	targets := append(found, files...)

	results := make([]HealthCheckResult, 0, len(targets))
	for _, path := range targets {
		res := HealthCheckResult{Path: path, Status: HealthStatusOK}
		if verifyErr := verifier.Verify(path); verifyErr != nil {
			res.Status = HealthStatusCorrupted
			res.Error = verifyErr.Error()
		}
		results = append(results, res)
	}
	return results, nil
}
