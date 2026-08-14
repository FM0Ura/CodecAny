package core

// MediaProber é a interface de inspeção de mídia (RF02, RI03).
// Implementações: pkg/adapters/ffmpeg (via ffprobe).
type MediaProber interface {
	// Probe extrai os metadados técnicos de um arquivo de mídia.
	Probe(path string) (MediaInfo, error)
}

// MediaVerifier é a interface de verificação de integridade de mídia (segurança).
// Implementações: pkg/adapters/ffmpeg (via decode test do ffmpeg).
type MediaVerifier interface {
	// Verify decodifica o arquivo de ponta a ponta e retorna erro se houver
	// qualquer falha de decodificação, garantindo que o arquivo é íntegro e
	// reproduzível antes de uma troca.
	Verify(path string) error
}

// TranscoderEngine é a interface de conversão (RF04, RI03).
// Implementações: pkg/adapters/ffmpeg (via ffmpeg).
type TranscoderEngine interface {
	// Name retorna o identificador do driver (ex: "ffmpeg").
	Name() string
	// Transcode converte o input em output aplicando o TargetSpec.
	// Retorna um canal de progresso (0.0 a 1.0) que é fechado ao concluir.
	Transcode(input, output string, media MediaInfo, target TargetSpec) (<-chan float64, error)
}
