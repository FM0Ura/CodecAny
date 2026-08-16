// Package logger centraliza o logging via log/slog, com saída em console e
// arquivo (rotação via lumberjack.v2) sob a pasta ./logs/.
package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultDir  = "logs"
	envVarLevel = "CODECANY_LOG_LEVEL"
)

// Config define as opções de criação do logger.
type Config struct {
	Dir         string // pasta dos arquivos de log (default "logs")
	JSONConsole bool   // se true, console emite JSON em vez de texto
	Level       string // debug|info|warn|error (default "info")
}

// New constrói um *slog.Logger que grava em ./<dir>/codecany.log (JSON,
// rotação por tamanho) e espelha no os.Stderr (texto ou JSON).
func New(cfg Config) (*slog.Logger, error) {
	if cfg.Dir == "" {
		cfg.Dir = defaultDir
	}
	level := parseLevel(cfg.Level)

	rot := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.Dir, "codecany.log"),
		MaxSize:    100, // MB
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}
	fileH := slog.NewJSONHandler(rot, &slog.HandlerOptions{Level: level})

	var consoleH slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if cfg.JSONConsole {
		consoleH = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		consoleH = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(slog.NewMultiHandler(fileH, consoleH)), nil
}

// Fatal registra a mensagem no logger e encerra o processo com código 1.
func Fatal(log *slog.Logger, err error) {
	log.Error("fatal", "error", err)
	os.Exit(1)
}

// parseLevel converte o nome textual no nível slog, respeitando a env var.
func parseLevel(s string) slog.Level {
	if s == "" {
		s = os.Getenv(envVarLevel)
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
