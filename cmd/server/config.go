package main

import (
	"flag"
	"strings"
)

// ServerConfig agrupa os parâmetros de configuração do servidor HTTP do
// painel de controle web (Fase A), espelhando os mesmos nomes de flag já
// usados em cmd/cli/main.go — mesma composição de Store+RulesEngine+Engine,
// só que num processo cliente diferente do mesmo pkg/core (ver
// docs/propostas_painel_controle.md, seção 1) — mais -addr, específico deste
// binário.
type ServerConfig struct {
	Addr         string
	DBPath       string
	RulesPath    string
	Workers      int
	LogDir       string
	LogLevel     string
	ScanInterval string
}

// multiFlag acumula valores repetidos de -dir. Duplicado de
// cmd/cli/main.go::multiFlag: os dois binários são pacotes main isolados
// (sem um pacote interno compartilhado só para este helper trivial de
// poucas linhas).
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// parseFlags parseia as flags de linha de comando do servidor a partir de
// args (tipicamente os.Args[1:], recebido como parâmetro em vez de usar o
// FlagSet global para permitir testar sem efeitos colaterais globais).
//
// -dir é repetível e opcional: a Fase A ainda não implementa a persistência
// de "diretórios monitorados" via API (isso é Fase C) — aceitar -dir aqui
// serve só para já ter algo rodando/testável localmente (mesmo padrão efêmero
// de cmd/cli, sem tocar o Store).
func parseFlags(args []string) (ServerConfig, []string, error) {
	var cfg ServerConfig
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	var d multiFlag

	fs.StringVar(&cfg.Addr, "addr", "127.0.0.1:8383", "endereço de bind do servidor HTTP")
	fs.StringVar(&cfg.DBPath, "db", "codecany.db", "arquivo SQLite (.db)")
	fs.StringVar(&cfg.RulesPath, "rules", "rules.yaml", "arquivo de regras YAML/JSON")
	fs.IntVar(&cfg.Workers, "workers", 1, "número de workers")
	fs.StringVar(&cfg.LogDir, "log-dir", "logs", "pasta dos arquivos de log")
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "nível de log (debug|info|warn|error)")
	fs.StringVar(&cfg.ScanInterval, "scan-interval", "1m", "intervalo da varredura periódica de fallback (ex.: 30s, 5m; 0 desliga)")
	fs.Var(&d, "dir", "diretório a monitorar (repita para vários; Fase A, efêmero — ver Fase C)")

	if err := fs.Parse(args); err != nil {
		return ServerConfig{}, nil, err
	}
	return cfg, d, nil
}
