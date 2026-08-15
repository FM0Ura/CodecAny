package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreGetTotalSavings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 1. Sem jobs completos, economia deve ser zero
	savings, err := store.GetTotalSavings()
	if err != nil {
		t.Fatal(err)
	}
	if savings.OriginalSizeBytes != 0 || savings.ConvertedSizeBytes != 0 || savings.SavedBytes != 0 {
		t.Errorf("esperava economia zero sem jobs, obteve: %+v", savings)
	}

	// 2. Cria dois jobs, um COMPLETED e outro FAILED
	job1 := &Job{
		ID:        "job-1",
		Path:      "movie1.mkv",
		Status:    StatusCompleted,
		Driver:    "ffmpeg",
		CreatedAt: time.Now(),
	}
	if err := store.CreateJob(job1); err != nil {
		t.Fatal(err)
	}
	// Atualiza as métricas do primeiro (sucesso)
	metrics1 := SizeMetrics{
		OriginalSizeBytes:   1000,
		ConvertedSizeBytes:  600,
		SavedBytes:          400,
		CompressionRatioPct: 40.0,
	}
	if err := store.UpdateMetrics(job1.ID, metrics1); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(job1.ID, StatusCompleted, nil, nil, ""); err != nil {
		t.Fatal(err)
	}

	job2 := &Job{
		ID:        "job-2",
		Path:      "movie2.mkv",
		Status:    StatusCompleted,
		Driver:    "ffmpeg",
		CreatedAt: time.Now(),
	}
	if err := store.CreateJob(job2); err != nil {
		t.Fatal(err)
	}
	// Atualiza as métricas do segundo (sucesso)
	metrics2 := SizeMetrics{
		OriginalSizeBytes:   2000,
		ConvertedSizeBytes:  1200,
		SavedBytes:          800,
		CompressionRatioPct: 40.0,
	}
	if err := store.UpdateMetrics(job2.ID, metrics2); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(job2.ID, StatusCompleted, nil, nil, ""); err != nil {
		t.Fatal(err)
	}

	job3 := &Job{
		ID:        "job-3",
		Path:      "movie3.mkv",
		Status:    StatusFailed,
		Driver:    "ffmpeg",
		CreatedAt: time.Now(),
	}
	if err := store.CreateJob(job3); err != nil {
		t.Fatal(err)
	}
	// Métricas de um job falho (não devem ser consideradas)
	metrics3 := SizeMetrics{
		OriginalSizeBytes:   5000,
		ConvertedSizeBytes:  5000,
		SavedBytes:          0,
		CompressionRatioPct: 0.0,
	}
	if err := store.UpdateMetrics(job3.ID, metrics3); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(job3.ID, StatusFailed, nil, nil, "some error"); err != nil {
		t.Fatal(err)
	}

	// 3. Testa economia consolidada
	savings, err = store.GetTotalSavings()
	if err != nil {
		t.Fatal(err)
	}

	expectedOriginal := int64(3000) // 1000 + 2000
	expectedConverted := int64(1800) // 600 + 1200
	expectedSaved := int64(1200) // 400 + 800
	expectedRatio := 40.0 // (1200 / 3000) * 100

	if savings.OriginalSizeBytes != expectedOriginal {
		t.Errorf("OriginalSizeBytes esperado %d, obteve %d", expectedOriginal, savings.OriginalSizeBytes)
	}
	if savings.ConvertedSizeBytes != expectedConverted {
		t.Errorf("ConvertedSizeBytes esperado %d, obteve %d", expectedConverted, savings.ConvertedSizeBytes)
	}
	if savings.SavedBytes != expectedSaved {
		t.Errorf("SavedBytes esperado %d, obteve %d", expectedSaved, savings.SavedBytes)
	}
	if savings.CompressionRatioPct != expectedRatio {
		t.Errorf("CompressionRatioPct esperado %v, obteve %v", expectedRatio, savings.CompressionRatioPct)
	}
}
