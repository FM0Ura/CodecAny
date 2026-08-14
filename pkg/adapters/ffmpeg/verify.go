package ffmpeg

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/FM0Ura/codecany/pkg/core"
)

// Verifier implementa core.MediaVerifier decodificando o arquivo de ponta a
// ponta com o ffmpeg e detectando erros via -v error.
type Verifier struct {
	bin string
}

// NewVerifier cria um verificador usando o ffmpeg do PATH.
func NewVerifier() *Verifier { return &Verifier{bin: "ffmpeg"} }

// WithBin permite apontar para um binário específico.
func (v *Verifier) WithBin(bin string) *Verifier { v.bin = bin; return v }

// Verify decodifica o arquivo inteiro sem produzir saída (-f null -). Qualquer
// erro de decodificação faz o ffmpeg reportar via stderr e retornar ≠ 0.
func (v *Verifier) Verify(path string) error {
	cmd := exec.Command(v.bin, "-v", "error", "-i", path, "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "saída vazia"
		}
		return fmt.Errorf("decode %s: %w: %s", path, err, msg)
	}
	return nil
}

var _ core.MediaVerifier = (*Verifier)(nil)
