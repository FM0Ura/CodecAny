package core

import (
	"fmt"
	"os"
)

// IntegrityCheck valida o output convertido e decide se o ganho de espaço é
// suficiente (RF05, seção 7). Efetua rollback se o ganho for irrelevante.
type IntegrityCheck struct {
	MinSavingPct float64
}

// NewIntegrityCheck cria um verificador com o limite mínimo de economia.
func NewIntegrityCheck(minSavingPct float64) *IntegrityCheck {
	return &IntegrityCheck{MinSavingPct: minSavingPct}
}

// ComputeSizeMetrics calcula as métricas de eficiência a partir dos dois tamanhos.
func ComputeSizeMetrics(originalSize, convertedSize int64) SizeMetrics {
	saved := originalSize - convertedSize
	ratio := 0.0
	if originalSize > 0 {
		ratio = float64(saved) / float64(originalSize) * 100
	}
	return SizeMetrics{
		OriginalSizeBytes:   originalSize,
		ConvertedSizeBytes:  convertedSize,
		SavedBytes:          saved,
		CompressionRatioPct: ratio,
	}
}

// Check avalia se o output deve ser mantido ou revertido (rollback).
// Retorna true se o output representa economia >= limite configurado.
//
// skipSavingsPolicy isenta o job das duas comparações de tamanho (limiar
// MinSavingPct e "output maior que original") — usado por jobs remux-only
// (video.codec: copy, Fase 3), cujo propósito é compatibilidade de
// container, não economia de espaço. As métricas ainda são computadas e
// retornadas normalmente para registro/relatório; a única guarda de
// segurança que continua se aplicando nesse caso é a verificação de
// decodificação via MediaVerifier.Verify (chamada pelo Engine antes de
// Check, não neste método).
func (c *IntegrityCheck) Check(originalPath, outputPath string, skipSavingsPolicy bool) (bool, SizeMetrics, error) {
	origInfo, err := os.Stat(originalPath)
	if err != nil {
		return false, SizeMetrics{}, fmt.Errorf("stat original: %w", err)
	}
	outInfo, err := os.Stat(outputPath)
	if err != nil {
		return false, SizeMetrics{}, fmt.Errorf("stat output: %w", err)
	}
	m := ComputeSizeMetrics(origInfo.Size(), outInfo.Size())

	if skipSavingsPolicy {
		return true, m, nil
	}

	// output maior que original → ganho negativo → rollback
	if m.ConvertedSizeBytes >= m.OriginalSizeBytes {
		return false, m, nil
	}
	// economia irrelevante (< limite) → rollback
	if m.CompressionRatioPct < c.MinSavingPct {
		return false, m, nil
	}
	return true, m, nil
}
