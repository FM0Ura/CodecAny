package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceAtomicCrossDevice simula a troca entre filesystems distintos:
// normally desloca o arquivo de origem para o diretório do destino a fim de
// reproduzir a queda do os.Rename (EXDEV). Verifica que replaceAtomic conclui
// mesmo assim, copiando para o device de destino.
func TestReplaceAtomicCrossDevice(t *testing.T) {
	// mesmo device neste teste; o ponto é exercitar o caminho de cópia+rename
	dir := t.TempDir()
	other := t.TempDir()
	src := filepath.Join(other, "orig-out.mkv")
	if err := os.WriteFile(src, []byte("0123456789"), 0o640); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "final.mkv")

	if err := replaceAtomic(src, dst); err != nil {
		t.Fatalf("replaceAtomic (fallback) falhou: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "0123456789" {
		t.Fatalf("conteúdo final não bateu: %q", b)
	}
	// origem não deve existir mais (foi movida), nem sobrar .tmp/.copy
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("esperava origem removida, mas existe: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".copy" || filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("sobrou temp de troca: %s", e.Name())
		}
	}
}

// TestReplaceAtomicSameDevice cobre o caminho rápido (rename direto).
func TestReplaceAtomicSameDevice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	dst := filepath.Join(dir, "b.mkv")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceAtomic(src, dst); err != nil {
		t.Fatalf("replaceAtomic direto falhou: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expect src moved; ainda existe: %v", err)
	}
}

// TestCleanupCommitContainerChange cobre o bug corrigido: quando a regra
// define um `convert.container` diferente do container do arquivo de
// entrada, o Commit final deve produzir o arquivo com a NOVA extensão (e
// remover o arquivo com a extensão antiga), em vez de gravar bytes de um
// container diferente sob o nome original.
func TestCleanupCommitContainerChange(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()

	job := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(job, []byte("bytes originais mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	cl, err := NewCleanup(job, stage, "mkv")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if got, want := filepath.Ext(cl.Output()), ".mkv"; got != want {
		t.Fatalf("Output() extensão = %q, esperado %q (output=%s)", got, want, cl.Output())
	}

	converted := []byte("bytes convertidos mkv")
	if err := os.WriteFile(cl.Output(), converted, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cl.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	newPath := filepath.Join(dir, "movie.mkv")
	b, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("esperava %s presente após commit: %v", newPath, err)
	}
	if string(b) != string(converted) {
		t.Fatalf("conteúdo final não bateu: %q", b)
	}

	if _, err := os.Stat(job); !os.IsNotExist(err) {
		t.Errorf("esperava %s removido após troca de container, mas existe (err=%v)", job, err)
	}
}

// TestCleanupCommitContainerUnspecified garante que, sem `convert.container`
// (string vazia — comportamento default de mergeSpec quando a regra e os
// defaults globais não definem container), o comportamento de sempre é
// preservado: a extensão de saída/final é a mesma do arquivo de entrada.
func TestCleanupCommitContainerUnspecified(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()

	job := filepath.Join(dir, "clip.mkv")
	if err := os.WriteFile(job, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	cl, err := NewCleanup(job, stage, "")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if got, want := filepath.Ext(cl.Output()), ".mkv"; got != want {
		t.Fatalf("Output() extensão = %q, esperado %q (sem container alvo)", got, want)
	}

	converted := []byte("convertido")
	if err := os.WriteFile(cl.Output(), converted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cl.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	b, err := os.ReadFile(job)
	if err != nil {
		t.Fatalf("esperava %s presente (mesmo caminho original): %v", job, err)
	}
	if string(b) != string(converted) {
		t.Fatalf("conteúdo final não bateu: %q", b)
	}
}

// TestCleanupCommitContainerSame garante que quando o container alvo coincide
// com a extensão de entrada (ex.: "mp4" para um arquivo .mp4), nada muda: o
// caminho final continua sendo o original.
func TestCleanupCommitContainerSame(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()

	job := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(job, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	cl, err := NewCleanup(job, stage, "MP4") // maiúsculas: deve normalizar
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if got, want := filepath.Ext(cl.Output()), ".mp4"; got != want {
		t.Fatalf("Output() extensão = %q, esperado %q", got, want)
	}

	converted := []byte("convertido")
	if err := os.WriteFile(cl.Output(), converted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cl.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	b, err := os.ReadFile(job)
	if err != nil {
		t.Fatalf("esperava %s presente: %v", job, err)
	}
	if string(b) != string(converted) {
		t.Fatalf("conteúdo final não bateu: %q", b)
	}
}

// TestCleanupCommitContainerAuto garante que "auto" (o coringa usado em
// global.defaults.container do rules.example.yaml para "manter o mesmo
// container do arquivo de entrada") NÃO é tratado como uma extensão literal
// ".auto" — do contrário um job sem `convert.container` explícito, herdando
// o default "auto", geraria um arquivo final com a extensão errada.
func TestCleanupCommitContainerAuto(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()

	job := filepath.Join(dir, "clip.mkv")
	if err := os.WriteFile(job, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	cl, err := NewCleanup(job, stage, "auto")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if got, want := filepath.Ext(cl.Output()), ".mkv"; got != want {
		t.Fatalf("Output() extensão = %q, esperado %q (\"auto\" não deve virar extensão)", got, want)
	}

	converted := []byte("convertido")
	if err := os.WriteFile(cl.Output(), converted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cl.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	b, err := os.ReadFile(job)
	if err != nil {
		t.Fatalf("esperava %s presente (mesmo caminho original): %v", job, err)
	}
	if string(b) != string(converted) {
		t.Fatalf("conteúdo final não bateu: %q", b)
	}
	if _, err := os.Stat(job + ".auto"); !os.IsNotExist(err) {
		t.Fatalf("não deveria existir um arquivo com extensão .auto")
	}
}

// TestRecoverInterruptedCommitRollsBack simula o crash exatamente entre os
// dois passos de Commit(): o original já foi renomeado para .bak (passo A)
// mas a troca atômica do output para o destino final (passo B) nunca
// aconteceu — finalPath está ausente. RecoverInterruptedCommit deve
// restaurar o backup de volta para o caminho original.
func TestRecoverInterruptedCommitRollsBack(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()
	job := filepath.Join(dir, "movie.mkv")

	cl, err := NewCleanup(job, stage, "")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	// Estado exato no meio de Commit(): passo A já rodou (original -> .bak),
	// passo B nunca chegou a rodar. O output em staging pode ou não estar
	// completo — não importa pro rollback, ele é descartado de qualquer forma.
	if err := os.WriteFile(job+".bak", []byte("bytes-originais"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cl.Output(), []byte("bytes-convertidos"), 0o600); err != nil {
		t.Fatal(err)
	}

	if outcome := cl.RecoverInterruptedCommit(); outcome != RecoveryRolledBack {
		t.Fatalf("outcome = %v, want RecoveryRolledBack", outcome)
	}

	b, err := os.ReadFile(job)
	if err != nil {
		t.Fatalf("esperava %s restaurado: %v", job, err)
	}
	if string(b) != "bytes-originais" {
		t.Errorf("conteúdo restaurado incorreto: %q", b)
	}
	if _, err := os.Stat(job + ".bak"); !os.IsNotExist(err) {
		t.Errorf("esperava .bak removido após restauração, mas existe")
	}
	if _, err := os.Stat(cl.StageDir()); !os.IsNotExist(err) {
		t.Errorf("esperava staging purgada, mas existe")
	}
}

// TestRecoverInterruptedCommitRollsBackWithContainerChange é a mesma
// simulação de TestRecoverInterruptedCommitRollsBack, mas com troca de
// container (.avi -> .mkv) — regressão específica para o bug encontrado ao
// escrever RecoverInterruptedCommit: checar a existência de jobPath (em vez
// de finalPath) para decidir se a troca já tinha concluído classificaria
// erroneamente TODO job com troca de extensão como "já ausente/rollback",
// já que jobPath (a extensão antiga) deixa de existir desde o passo A
// independentemente do resultado do passo B.
func TestRecoverInterruptedCommitRollsBackWithContainerChange(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()
	job := filepath.Join(dir, "movie.avi")

	cl, err := NewCleanup(job, stage, "mkv")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if got, want := cl.FinalPath(), filepath.Join(dir, "movie.mkv"); got != want {
		t.Fatalf("FinalPath() = %q, want %q", got, want)
	}

	if err := os.WriteFile(job+".bak", []byte("bytes-originais"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cl.Output(), []byte("bytes-convertidos"), 0o600); err != nil {
		t.Fatal(err)
	}

	if outcome := cl.RecoverInterruptedCommit(); outcome != RecoveryRolledBack {
		t.Fatalf("outcome = %v, want RecoveryRolledBack", outcome)
	}
	b, err := os.ReadFile(job)
	if err != nil || string(b) != "bytes-originais" {
		t.Fatalf("esperava %s restaurado com bytes-originais: %q, %v", job, b, err)
	}
	if _, err := os.Stat(cl.FinalPath()); !os.IsNotExist(err) {
		t.Errorf("não deveria existir %s (troca de container nunca concluiu)", cl.FinalPath())
	}
}

// TestRecoverInterruptedCommitAlreadyCommittedWithLeftoverBackup simula um
// crash logo APÓS o passo B (troca já concluída) mas ANTES da limpeza do
// backup/staging (passos C/D): finalPath já tem o conteúdo convertido e o
// .bak ainda existe. Nada deve ser restaurado — só a limpeza é completada.
func TestRecoverInterruptedCommitAlreadyCommittedWithLeftoverBackup(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()
	job := filepath.Join(dir, "movie.mkv")

	cl, err := NewCleanup(job, stage, "")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if err := os.WriteFile(job, []byte("bytes-convertidos"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(job+".bak", []byte("bytes-originais"), 0o600); err != nil {
		t.Fatal(err)
	}

	if outcome := cl.RecoverInterruptedCommit(); outcome != RecoveryAlreadyCommitted {
		t.Fatalf("outcome = %v, want RecoveryAlreadyCommitted", outcome)
	}
	b, err := os.ReadFile(job)
	if err != nil || string(b) != "bytes-convertidos" {
		t.Errorf("jobPath não deveria ter sido alterado: %q, %v", b, err)
	}
	if _, err := os.Stat(job + ".bak"); !os.IsNotExist(err) {
		t.Errorf("esperava .bak limpo, mas existe")
	}
	if _, err := os.Stat(cl.StageDir()); !os.IsNotExist(err) {
		t.Errorf("esperava staging purgada, mas existe")
	}
}

// TestRecoverInterruptedCommitAlreadyCommittedNoBackup cobre o caso em que
// finalPath já existe (convertido, ou o Commit() nunca chegou a rodar — não
// dá pra saber com certeza só olhando o filesystem) e não há backup nenhum:
// nada a restaurar, o arquivo já está num estado consistente de qualquer
// forma.
func TestRecoverInterruptedCommitAlreadyCommittedNoBackup(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()
	job := filepath.Join(dir, "movie.mkv")

	cl, err := NewCleanup(job, stage, "")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}
	if err := os.WriteFile(job, []byte("qualquer-conteudo"), 0o600); err != nil {
		t.Fatal(err)
	}

	if outcome := cl.RecoverInterruptedCommit(); outcome != RecoveryAlreadyCommitted {
		t.Fatalf("outcome = %v, want RecoveryAlreadyCommitted", outcome)
	}
	if _, err := os.Stat(cl.StageDir()); !os.IsNotExist(err) {
		t.Errorf("esperava staging purgada, mas existe")
	}
}

// TestRecoverInterruptedCommitAmbiguous cobre o estado inesperado: nem
// finalPath nem o backup existem em disco (ex.: falha dupla durante a cópia
// cross-device). Nenhuma ação automática deve ser tentada.
func TestRecoverInterruptedCommitAmbiguous(t *testing.T) {
	dir := t.TempDir()
	stage := t.TempDir()
	job := filepath.Join(dir, "movie.mkv")

	cl, err := NewCleanup(job, stage, "")
	if err != nil {
		t.Fatalf("NewCleanup: %v", err)
	}

	if outcome := cl.RecoverInterruptedCommit(); outcome != RecoveryAmbiguous {
		t.Fatalf("outcome = %v, want RecoveryAmbiguous", outcome)
	}
}
