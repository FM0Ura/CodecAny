package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Cleanup rastreia os resíduos de um Job e garante a remoção via Defer Pattern
// (Zero-Residue Policy, seção 5) mesmo em caso de falha.
type Cleanup struct {
	jobPath   string
	finalPath string
	stageDir  string
	output    string
	localBak  string
	tmpFiles  []string
	started   bool
	committed bool
}

// NewCleanup prepara a pasta de staging e caminhos temporários de um Job.
// O backup é criado na MESMA pasta do original para permitir troca atômica.
//
// targetContainer é o `convert.container` já resolvido da regra (TargetSpec.
// Container). Quando vazio, ou quando normaliza para a mesma extensão do
// arquivo de entrada, o comportamento é o de sempre: o output em staging e o
// caminho final reaproveitam a extensão original. Quando difere, tanto o
// output em staging quanto o caminho final (finalPath) passam a usar a nova
// extensão — do contrário o arquivo trocado ficaria com bytes de um
// container diferente do nome que carrega.
func NewCleanup(jobPath, stageRoot, targetContainer string) (*Cleanup, error) {
	inExt := filepath.Ext(jobPath)
	base := strings.TrimSuffix(filepath.Base(jobPath), inExt)
	stage := filepath.Join(stageRoot, base)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return nil, fmt.Errorf("criar staging: %w", err)
	}

	outExt := inExt
	finalPath := jobPath
	if norm := normalizeContainerExt(targetContainer); norm != "" && norm != strings.ToLower(strings.TrimPrefix(inExt, ".")) {
		outExt = "." + norm
		finalPath = filepath.Join(filepath.Dir(jobPath), base+outExt)
	}

	return &Cleanup{
		jobPath:   jobPath,
		finalPath: finalPath,
		stageDir:  stage,
		output:    filepath.Join(stage, "output"+outExt),
		localBak:  jobPath + ".bak",
	}, nil
}

// normalizeContainerExt reduz um valor de `convert.container` (ex.: "mp4",
// "MKV", ".webm") a uma extensão em minúsculas sem o ponto. Vazio ou "auto"
// significam "sem container alvo definido" (nenhuma troca de extensão deve
// ocorrer) — "auto" é o coringa usado em rules.go/rules.example.yaml para
// "manter o mesmo container do arquivo de entrada", não um nome de extensão.
func normalizeContainerExt(container string) string {
	c := strings.ToLower(strings.TrimSpace(container))
	if c == "auto" {
		return ""
	}
	return strings.TrimPrefix(c, ".")
}

// StageDir retorna o caminho da pasta de trabalho temporária.
func (c *Cleanup) StageDir() string { return c.stageDir }

// Output retorna o caminho do arquivo convertido em staging.
func (c *Cleanup) Output() string { return c.output }

// FinalPath retorna o caminho onde o arquivo convertido ficará após um
// Commit bem-sucedido. É igual a jobPath quando o container alvo não muda a
// extensão; caso contrário, é o novo caminho (nova extensão) que substitui o
// original — o chamador deve atualizar qualquer referência persistida ao
// caminho do job (Job.Path, eventos, webhooks, logs) para este valor após o
// Commit, já que o caminho antigo deixa de existir no disco.
func (c *Cleanup) FinalPath() string { return c.finalPath }

// Track registra um arquivo temporário para purga.
func (c *Cleanup) Track(path string) { c.tmpFiles = append(c.tmpFiles, path) }

// Commit realiza a troca atômica do output no lugar do original e purga resíduos.
// Sequência: renomeia original → .bak local, output → finalPath, remove .bak
// e remove toda a staging. (FINALIZING bem-sucedido)
//
// Quando o container alvo mudou a extensão (finalPath != jobPath), o arquivo
// original permanece de fora como .bak até a troca atômica ter sucesso e só
// então é removido — assim o arquivo com a extensão antiga deixa de existir
// e sobra apenas finalPath (nova extensão) com o conteúdo convertido. Se
// finalPath já existir como resíduo de uma execução anterior, replaceAtomic
// o substitui com a mesma semântica de troca atômica usada no caso normal.
//
// O staging pode ficar em outro filesystem que o original; nesse caso o rename
// cruzaria devices (EXDEV) e falharia, então há fallback que copia o output
// para um temporário na própria pasta do original (mesmo device) antes da troca.
func (c *Cleanup) Commit() error {
	if _, err := os.Stat(c.output); err != nil {
		return fmt.Errorf("output ausente: %w", err)
	}
	if err := os.Rename(c.jobPath, c.localBak); err != nil {
		return fmt.Errorf("backup original: %w", err)
	}
	if err := replaceAtomic(c.output, c.finalPath); err != nil {
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

// replaceAtomic substitui dst por src de forma atômica quando possível. Se os
// arquivos estão em filesystems diferentes (EXDEV), copia src para um temporário
// no diretório de dst (mesmo device) e então renomeia — a troca final fica atômica.
func replaceAtomic(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	target, err := copyFile(src, filepath.Dir(dst), copyName(filepath.Base(dst)))
	if err != nil {
		return fmt.Errorf("copiar para troca: %w", err)
	}
	if err := os.Rename(target, dst); err != nil {
		os.Remove(target)
		return fmt.Errorf("troca no device local: %w", err)
	}
	return nil
}

// copyName gera o nome de temp usado durante a troca cross-device.
func copyName(base string) string {
	return "." + base + ".codecany.tmp"
}

// copyFile copia o conteúdo (preservando permissões) de src para destDir/destName.
func copyFile(src, destDir, destName string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return "", err
	}
	out := filepath.Join(destDir, destName)
	tmp := out + ".copy"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, in); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return out, nil
}

// CommitFiles é um alias de Commit para uso em testes da engine.
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
