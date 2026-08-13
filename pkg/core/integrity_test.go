package core

import (
	"os"
	"testing"
)

func TestIntegrityCheckKeepsWhenSaving(t *testing.T) {
	dir := t.TempDir()
	orig := writeFileSize(t, dir+"/orig.mkv", 1000)
	out := writeFileSize(t, dir+"/out.mkv", 700) // 30% economia
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	c := NewIntegrityCheck(15)
	keep, m, err := c.Check(orig, out)
	if err != nil {
		t.Fatal(err)
	}
	if !keep {
		t.Fatal("esperava manter (30% >= 15%)")
	}
	if m.SavedBytes != 300 {
		t.Errorf("saved esperado 300, obteve %d", m.SavedBytes)
	}
	if m.CompressionRatioPct != 30 {
		t.Errorf("ratio esperado 30, obteve %.2f", m.CompressionRatioPct)
	}
}

func TestIntegrityCheckRollbackWhenInsufficient(t *testing.T) {
	dir := t.TempDir()
	orig := writeFileSize(t, dir+"/orig.mkv", 1000)
	out := writeFileSize(t, dir+"/out.mkv", 950) // 5% < 15%
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	c := NewIntegrityCheck(15)
	keep, _, err := c.Check(orig, out)
	if err != nil {
		t.Fatal(err)
	}
	if keep {
		t.Fatal("esperava rollback (5% < 15%)")
	}
}

func TestIntegrityCheckRollbackWhenBigger(t *testing.T) {
	dir := t.TempDir()
	orig := writeFileSize(t, dir+"/orig.mkv", 1000)
	out := writeFileSize(t, dir+"/out.mkv", 2000)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	c := NewIntegrityCheck(15)
	keep, _, err := c.Check(orig, out)
	if err != nil {
		t.Fatal(err)
	}
	if keep {
		t.Fatal("output maior que original deve dar rollback")
	}
}
