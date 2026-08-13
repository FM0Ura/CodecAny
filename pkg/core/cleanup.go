package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Cleanup rastreia os resíduos de um Job e garante a remoção via Defer Pattern
// (Zero-Residue Policy, seção 5) mesmo em caso de falha.
type Cleanup struct {
	jobPath   string
	stageDir  string
	output    string
	localBak  string
	tmpFiles  []string
	started   bool
	committed bool
}

// NewCleanup prepara a pasta de staging e caminhos temporários de um Job.
// O backup é criado na MESMA pasta do original para permitir troca atômica.
func NewCleanup(jobPath, stageRoot string) (*Cleanup, error) {
	base := strings.TrimSuffix(filepath.Base(jobPath), filepath.Ext(jobPath))
	stage := filepath.Join(stageRoot, base)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return nil, fmt.Errorf("criar staging: %w", err)
	}
	return &Cleanup{
		jobPath:  jobPath,
		stageDir: stage,
		output:   filepath.Join(stage, "output"+filepath.Ext(jobPath)),
		localBak: jobPath + ".bak",
	}, nil
}

// StageDir retorna o caminho da pasta de trabalho temporária.
func (c *Cleanup) StageDir() string { return c.stageDir }

// Output retorna o caminho do arquivo convertido em staging.
func (c *Cleanup) Output() string { return c.output }

// Track registra um arquivo temporário para purga.
func (c *Cleanup) Track(path string) { c.tmpFiles = append(c.tmpFiles, path) }

// Commit realiza a troca atômica do output no lugar do original e purga resíduos.
// Sequência: renomeia original → .bak local, output → original, remove .bak
// e remove toda a staging. (FINALIZING bem-sucedido)
func (c *Cleanup) Commit() error {
	if _, err := os.Stat(c.output); err != nil {
		return fmt.Errorf("output ausente: %w", err)
	}
	if err := os.Rename(c.jobPath, c.localBak); err != nil {
		return fmt.Errorf("backup original: %w", err)
	}
	if err := os.Rename(c.output, c.jobPath); err != nil {
		os.Rename(c.localBak, c.jobPath) // reverte
		return fmt.Errorf("substituição atômica: %w", err)
	}
	os.Remove(c.localBak)
	os.RemoveAll(c.stageDir)
	c.committed = true
	return nil
}

// Abort restaura o original (se necessário), purga temporários e staging.
// Chamado em FAILED / ROLLED_BACK (seções 5.1/5.2).
func (c *Cleanup) Abort() error {
	var errs []string
	if _, err := os.Stat(c.localBak); err == nil {
		if err := os.Rename(c.localBak, c.jobPath); err != nil {
			errs = append(errs, fmt.Sprintf("restaurar original: %v", err))
		}
	}
	for _, f := range c.tmpFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("purga %s: %v", f, err))
		}
	}
	if err := os.RemoveAll(c.stageDir); err != nil {
		errs = append(errs, fmt.Sprintf("purga staging: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("cleanup: %v", errs)
	}
	return nil
}

// CommitFiles é um alias de Commit para uso em testes/lengton da engine.
func (c *Cleanup) CommitFiles(outputPath string) error {
	_ = outputPath
	return c.Commit()
}

// BootPurge repara resíduos abandonados por crashes (seção 5.3): restaura
// qualquer .bak e remove staging/tmp órfãos.
func BootPurge(stageRoot string) error {
	return filepath.Walk(stageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		entries, _ := os.ReadDir(path)
		var bakPath string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".bak") {
				bakPath = filepath.Join(path, e.Name())
			} else if strings.HasSuffix(e.Name(), ".tmp") {
				os.Remove(filepath.Join(path, e.Name()))
			}
		}
		if bakPath != "" {
			// stagingDir = stageRoot/<base>/ e original = <stageRoot>/<base><ext>
			base := filepath.Base(path)
			ext := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(bakPath), "original"), ".bak")
			orig := filepath.Join(filepath.Dir(path), base+ext)
			os.Rename(bakPath, orig)
		}
		return os.RemoveAll(path)
	})
}
