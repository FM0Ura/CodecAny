package ffmpeg

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/FM0Ura/codecany/pkg/core"
)

// Transcode implements core.TranscoderEngine usando o binário ffmpeg.
// Traduz o TargetSpec em flags nativas (seção 4.4) e parseia o progresso
// do -progress pipe: em tempo real (RF04).
type Transcode struct {
	bin string
}

// NewTranscode cria um transcoder usando o ffmpeg do PATH.
func NewTranscode() *Transcode { return &Transcode{bin: "ffmpeg"} }

// WithBin permite apontar para um binário específico.
func (t *Transcode) WithBin(bin string) *Transcode { t.bin = bin; return t }

// Name retorna o identificador do driver (RF04).
func (t *Transcode) Name() string { return "ffmpeg" }

func isHeavyLosslessAudio(codec string) bool {
	codec = strings.ToLower(codec)
	return codec == "truehd" ||
		strings.HasPrefix(codec, "dts") ||
		strings.HasPrefix(codec, "pcm_") ||
		codec == "lpcm"
}

// isMP4 decide se o alvo de conversão é um container MP4, seja pelo
// TargetSpec.Container explícito ou pela extensão do arquivo de saída.
func isMP4(target core.TargetSpec, output string) bool {
	return target.Container == "mp4" || strings.HasSuffix(strings.ToLower(output), ".mp4")
}

// isImageSubtitleCodec identifica codecs de legenda BASEADOS EM IMAGEM
// (bitmap), que nenhum player de mov_text consegue interpretar e que o
// container MP4 também não sabe carregar via "copy". Legendas de texto
// (subrip/ass/webvtt/mov_text...) retornam false.
func isImageSubtitleCodec(codec string) bool {
	switch strings.ToLower(codec) {
	case "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle":
		return true
	default:
		return false
	}
}

// buildVideoMapArgs monta as flags "-map" para os streams de vídeo do input
// principal, garantindo que o stream de vídeo "real" (identificado pelo
// Probe, ignorando capas/attached_pic) seja SEMPRE o primeiro a ser mapeado
// — e portanto sempre o índice de saída 0, que é o índice que buildArgs()
// mira com "-c:v:0" ao aplicar o codec/CRF/preset alvo. Eventuais streams de
// capa (attached_pic) são mapeados em seguida e permanecem no "-c:v copy"
// padrão de buildArgs (nunca recebem o codec/CRF do vídeo real).
//
// Isso corrige o bug de usar "-map 0:v" (mapeamento em bloco, que preserva a
// ORDEM ORIGINAL dos streams do container): se a capa aparecer antes do
// vídeo real no arquivo de origem, "-map 0:v" faria a capa cair no índice de
// saída 0 e "-c:v:0" acabaria recodificando a capa com parâmetros de vídeo
// de movimento, deixando o vídeo real intocado como "copy".
func buildVideoMapArgs(media core.MediaInfo) []string {
	var args []string
	if media.HasVideo {
		args = append(args, "-map", fmt.Sprintf("0:v:%d", media.VideoStreamIndex))
	}
	for _, idx := range media.CoverArtStreamIndexes {
		args = append(args, "-map", fmt.Sprintf("0:v:%d", idx))
	}
	return args
}

// buildSubtitleMapArgs monta as flags "-map" para os streams de legenda
// EMBUTIDOS no container de origem (não inclui os arquivos avulsos de
// media.SubtitlePaths, que são sempre texto e mapeados separadamente).
//
// Ao gerar MP4, streams de legenda baseados em imagem (PGS/VobSub/DVB) são
// DESCARTADOS do mapeamento: o MP4 não suporta esses codecs nativamente
// ("copy" falha) e mov_text é um formato de texto (não consegue representar
// bitmaps), então mapeá-los faria o ffmpeg abortar o job inteiro. Fora do
// MP4, todos os streams de legenda são mantidos — comportamento idêntico ao
// anterior ("-map 0:s?").
func buildSubtitleMapArgs(subtitleCodecs []string, isMP4 bool) []string {
	var args []string
	for i, codec := range subtitleCodecs {
		if isMP4 && isImageSubtitleCodec(codec) {
			continue
		}
		args = append(args, "-map", fmt.Sprintf("0:s:%d", i))
	}
	return args
}

// hwaccelEncoders mapeia (vendor de hwaccel, em minúsculas) -> (codec base
// solicitado -> encoder ffmpeg específico de hardware). É a fonte única de
// verdade usada tanto por buildArgs() (para escolher o encoder) quanto por
// Transcode() (para decidir se a flag "-hwaccel" deve ser emitida) — assim
// os dois lugares nunca discordam sobre o que é um vendor/codec reconhecido.
var hwaccelEncoders = map[string]map[string]string{
	"nvenc": {"hevc": "hevc_nvenc", "h264": "h264_nvenc", "av1": "av1_nvenc"},
	"cuda":  {"hevc": "hevc_nvenc", "h264": "h264_nvenc", "av1": "av1_nvenc"},
	"vaapi": {"hevc": "hevc_vaapi", "h264": "h264_vaapi", "av1": "av1_vaapi"},
	"qsv":   {"hevc": "hevc_qsv", "h264": "h264_qsv", "av1": "av1_qsv"},
	// videotoolbox (macOS) não tem encoder av1 oficial no ffmpeg — ausência
	// proposital, não esquecimento.
	"videotoolbox": {"hevc": "hevc_videotoolbox", "h264": "h264_videotoolbox"},
}

// resolveHWAccelEncoder busca o encoder de hardware para o par
// (vendor de hwaccel, codec base pedido pela regra: "hevc"/"h264"/"av1").
// O segundo retorno (recognized) indica se o vendor é conhecido E suporta o
// codec base pedido; quando false, o chamador deve cair para o encoder por
// software E não emitir "-hwaccel", pois um valor de vendor não reconhecido
// (ex.: typo "nvidia") faria o ffmpeg rejeitar a flag "-hwaccel" mesmo já
// tendo caído para software.
func resolveHWAccelEncoder(hw, codecBase string) (encoder string, recognized bool) {
	vendor, ok := hwaccelEncoders[strings.ToLower(hw)]
	if !ok {
		return "", false
	}
	enc, ok := vendor[codecBase]
	if !ok {
		return "", false
	}
	return enc, true
}

// softwareFallbackCodec retorna o encoder por software padrão para um codec
// base, usado tanto quando nenhum hwaccel foi pedido quanto quando o vendor
// pedido não foi reconhecido/suportado (fallback silencioso).
func softwareFallbackCodec(codecBase string, lossless bool) string {
	switch codecBase {
	case "hevc":
		return "libx265"
	case "h264":
		return "libx264"
	case "av1":
		if lossless {
			return "libaom-av1"
		}
		return "libsvtav1"
	default:
		return codecBase
	}
}

// buildArgs monta os argumentos do ffmpeg a partir do TargetSpec.
//
// srcHeight é a altura do vídeo de origem (MediaInfo.Height), usada apenas
// para decidir se um downscale via "-vf scale=-2:<max_height>" deve ser
// aplicado quando target.VideoMaxHeight>0. Este é o PRIMEIRO filtro de vídeo
// do projeto — até aqui buildArgs nunca emitia "-vf"/"-filter:v" para nenhum
// caso; a introdução deste mecanismo é intencional e abre caminho para
// filtros futuros (crop, deinterlace, tonemap HDR->SDR) sem exigir que sejam
// implementados agora.
func buildArgs(target core.TargetSpec, isMP4 bool, audioCodecs []string, srcHeight int) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-stats_period", "0.1"}

	if target.VideoCodec != "" {
		codec := target.VideoCodec

		if target.VideoHWAccel != "" {
			hw := strings.ToLower(target.VideoHWAccel)
			if enc, recognized := resolveHWAccelEncoder(hw, codec); recognized {
				codec = enc
			} else if codec == "hevc" || codec == "h264" || codec == "av1" {
				// Vendor não reconhecido (ex.: typo "nvidia") ou reconhecido
				// mas sem suporte a este codec base (ex.: videotoolbox+av1):
				// cai silenciosamente para o encoder por software — mesmo
				// resultado do ramo "sem hwaccel" abaixo. Transcode() usa a
				// mesma tabela (resolveHWAccelEncoder) para saber que o
				// hwaccel NÃO foi aplicado e evitar emitir "-hwaccel".
				codec = softwareFallbackCodec(codec, target.VideoLossless)
			}
		} else if codec == "hevc" || codec == "av1" {
			// Mapeia codecs genéricos para encoders recomendados em software
			codec = softwareFallbackCodec(codec, target.VideoLossless)
		}

		if codec == "copy" {
			args = append(args, "-c:v", "copy")
		} else {
			args = append(args, "-c:v", "copy", "-c:v:0", codec)
		}

		if codec != "copy" {
			// Preset
			preset := target.VideoPreset
			if target.VideoLossless && strings.HasSuffix(codec, "_nvenc") {
				preset = ""
			}
			if preset == "" {
				if codec == "libx265" {
					preset = "slow"
				} else if codec == "libsvtav1" {
					preset = "5"
				} else if codec == "libaom-av1" {
					preset = "4"
				} else if strings.HasSuffix(codec, "_nvenc") {
					if !target.VideoLossless {
						preset = "p5"
					}
				}
			}
			if preset != "" {
				args = append(args, "-preset", preset)
			}

			// Lógica de Sem Perdas (Lossless) vs Com Perdas (Lossy)
			if target.VideoLossless {
				if codec == "libx265" {
					args = append(args, "-x265-params", "lossless=1:open-gop=0")
				} else if codec == "libaom-av1" {
					args = append(args, "-crf", "0", "-aom-params", "lossless=1")
				} else if strings.HasSuffix(codec, "_nvenc") {
					args = append(args, "-preset", "lossless")
				} else if strings.HasSuffix(codec, "_vaapi") {
					args = append(args, "-qp", "0")
				} else if strings.HasSuffix(codec, "_qsv") {
					args = append(args, "-global_quality", "0")
				} else {
					args = append(args, "-crf", "0")
				}
			} else {
				// Com perdas (Lossy)
				crf := target.VideoCRF
				if crf == 0 {
					if codec == "libx265" {
						crf = 20
					} else if codec == "libsvtav1" {
						crf = 28
					} else if strings.HasSuffix(codec, "_nvenc") {
						crf = 23
					}
				}
				if crf > 0 {
					if strings.HasSuffix(codec, "_nvenc") {
						args = append(args, "-rc", "vbr", "-cq", strconv.Itoa(crf))
					} else if strings.HasSuffix(codec, "_vaapi") || strings.HasSuffix(codec, "_qsv") {
						args = append(args, "-global_quality", strconv.Itoa(crf))
					} else if strings.HasSuffix(codec, "_videotoolbox") {
						args = append(args, "-q:v", strconv.Itoa(crf))
					} else {
						args = append(args, "-crf", strconv.Itoa(crf))
					}
				}

				// pix_fmt yuv420p10le para evitar color banding
				if codec == "libx265" || codec == "libsvtav1" || strings.HasSuffix(codec, "_nvenc") {
					args = append(args, "-pix_fmt", "yuv420p10le")
				}

				// Parâmetros específicos
				if codec == "libx265" {
					args = append(args, "-x265-params", "open-gop=0")
				} else if codec == "libsvtav1" {
					tuneVal := "0"
					if target.VideoTune != "" {
						tuneVal = target.VideoTune
					}
					args = append(args, "-svtav1-params", fmt.Sprintf("tune=%s", tuneVal))
				}

				if target.VideoTune != "" && codec != "libsvtav1" {
					args = append(args, "-tune", target.VideoTune)
				}
			}

			// Downscale declarativo (video.max_height): só se aplica quando há
			// de fato recodificação (codec!="copy" — remux não deve escalar,
			// já que "-c:v copy" não permite filtros de vídeo) e a origem é
			// mais alta que o teto configurado. "-2" na largura mantém o
			// aspect ratio e força um valor par (exigido por vários encoders).
			if codec != "copy" && target.VideoMaxHeight > 0 && srcHeight > target.VideoMaxHeight {
				args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", target.VideoMaxHeight))
			}
		}
	}

	if target.AudioCodec != "" {
		if target.AudioCodec == "flac" && len(audioCodecs) > 0 {
			for idx, codecName := range audioCodecs {
				if isHeavyLosslessAudio(codecName) {
					args = append(args, fmt.Sprintf("-c:a:%d", idx), "flac")
				} else {
					args = append(args, fmt.Sprintf("-c:a:%d", idx), "copy")
				}
			}
		} else {
			args = append(args, "-c:a", target.AudioCodec)
			if target.AudioBitrate != "" && target.AudioCodec != "copy" {
				args = append(args, "-b:a", target.AudioBitrate)
			}
		}
	}
	// Se nenhum codec de áudio foi especificado (nem pela regra, nem por
	// default), omite a flag -c:a e deixa o ffmpeg escolher um encoder
	// compatível com o container de saída. Forçar "copy" aqui falha quando o
	// codec de origem é incompatível com o container alvo (ex.: DTS/TrueHD → MP4).

	// Sempre copia legendas (com mapping específico se for MP4)
	if isMP4 {
		args = append(args, "-c:s", "mov_text")
	} else {
		args = append(args, "-c:s", "copy")
	}

	return args
}

// buildTranscodeArgs monta a lista completa de argumentos do ffmpeg para um
// job de transcodificação: decisão de "-hwaccel", mapeamento de streams
// (vídeo/áudio/legendas embutidas/legendas avulsas/anexos) e as flags de
// codec/CRF/preset produzidas por buildArgs(). Extraído de Transcode() como
// função pura (sem side effects de exec.Command) para permitir testar a
// construção dos argumentos sem precisar de um binário ffmpeg real.
func buildTranscodeArgs(input, output string, media core.MediaInfo, target core.TargetSpec) []string {
	mp4 := isMP4(target, output)

	var args []string
	if target.VideoHWAccel != "" {
		hw := strings.ToLower(target.VideoHWAccel)
		// Usa a MESMA tabela que buildArgs() usa para escolher o encoder:
		// só emitimos "-hwaccel" quando o vendor foi de fato reconhecido e
		// aplicado. Se buildArgs() caiu para software (vendor desconhecido,
		// ex.: typo "nvidia", ou vendor sem suporte a este codec base), não
		// emitimos "-hwaccel" — caso contrário o ffmpeg recebe um encoder de
		// software junto com uma flag "-hwaccel" que ele não reconhece e
		// rejeita o comando inteiro (bug: fallback silencioso virava falha).
		if _, recognized := resolveHWAccelEncoder(hw, target.VideoCodec); recognized {
			// Alias: a flag "-hwaccel" (decodificação) do ffmpeg usa "cuda",
			// não "nvenc" (que é o nome do encoder, usado só em "-c:v").
			hwFlag := hw
			if hwFlag == "nvenc" {
				hwFlag = "cuda"
			}
			args = append(args, "-hwaccel", hwFlag)
		}
	}
	args = append(args, "-i", input)

	for _, sub := range media.SubtitlePaths {
		args = append(args, "-i", sub)
	}

	// Vídeo: mapeamento explícito por índice (não "-map 0:v" em bloco) para
	// garantir que o vídeo real caia sempre no índice de saída 0 — alvo do
	// "-c:v:0" em buildArgs() — mesmo que uma capa/attached_pic apareça
	// antes dele no container de origem (bug de mis-seleção do stream).
	args = append(args, buildVideoMapArgs(media)...)
	if media.HasAudio {
		args = append(args, "-map", "0:a")
	}
	// Legendas embutidas: mapeadas por índice para permitir descartar, ao
	// gerar MP4, as baseadas em imagem (PGS/VobSub/DVB) que quebrariam o job.
	args = append(args, buildSubtitleMapArgs(media.SubtitleCodecs, mp4)...)
	args = append(args, "-map", "0:t?")

	for idx := range media.SubtitlePaths {
		args = append(args, "-map", fmt.Sprintf("%d:s", idx+1))
	}

	// Preserva todos os metadados globais do arquivo de entrada (seção 10.1 do guia)
	args = append(args, "-map_metadata", "0")

	args = append(args, buildArgs(target, mp4, media.AudioCodecs, media.Height)...)
	args = append(args, output)

	return args
}

// Transcode executa o ffmpeg e emite progresso (0..1) no canal retornado.
func (t *Transcode) Transcode(input, output string, media core.MediaInfo, target core.TargetSpec) (<-chan float64, error) {
	args := buildTranscodeArgs(input, output, media, target)

	cmd := exec.Command(t.bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	prog := make(chan float64, 128)
	go func() {
		defer close(prog)
		readProgress(stdout, media.DurationSec, media.FrameRate, prog)
		cmd.Wait()
	}()

	return prog, nil
}

// readProgress lê as linhas de "-progress pipe:1" de r e emite frações de
// progresso (0..1) em out, usando durationSec como referência primária.
// Extraída de Transcode() como função pura (recebe um io.Reader genérico em
// vez de depender de cmd.StdoutPipe()) para ser testável sem precisar de um
// processo ffmpeg real.
//
// frameRate (quadros/segundo, ver core.MediaInfo.FrameRate) alimenta um
// fallback pelo número de frames: em containers com múltiplos streams de
// saída (vídeo real + capa + várias faixas de áudio + legendas), o ffmpeg
// pode reportar "out_time"/"out_time_ms" como "N/A" durante TODA a
// conversão mesmo com "frame=" avançando normalmente — sem esse fallback a
// fração calculada (outTime/durationSec) fica travada em 0 do início ao
// fim, dando a impressão de que a barra de progresso não funciona. Quando
// frameRate<=0 (não foi possível determinar no probe), o fallback nunca
// dispara e o comportamento é o mesmo de antes (só o "1.0" final de
// progress=end).
func readProgress(r io.Reader, durationSec, frameRate float64, out chan<- float64) {
	sc := bufio.NewScanner(r)
	outTime := 0.0
	haveOutTime := false
	frameNum := 0.0
	totalFrames := frameRate * durationSec

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "frame="):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(line, "frame="), 64); err == nil {
				frameNum = v
			}
		case strings.HasPrefix(line, "out_time_ms="):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_ms="), 64); err == nil {
				outTime = v / 1e6
				haveOutTime = true
			}
		case strings.HasPrefix(line, "out_time="):
			if d, err := parseOutTime(strings.TrimPrefix(line, "out_time=")); err == nil {
				outTime = d
				haveOutTime = true
			}
		case line == "progress=end":
			out <- 1.0
			// Não cai no bloco de reenvio abaixo: sem o "continue", a
			// fração calculada a partir do outTime (possivelmente
			// desatualizado nesta última linha) seria reenviada logo depois
			// do 1.0 já emitido, fazendo a barra "voltar" momentaneamente
			// (ex.: 100% seguido de 90%) bem no fim da conversão.
			continue
		}
		switch {
		case haveOutTime && durationSec > 0:
			if r := outTime / durationSec; r >= 0 && r <= 1 {
				out <- r
			}
		case totalFrames > 0:
			if r := frameNum / totalFrames; r >= 0 && r <= 1 {
				out <- r
			}
		}
	}
}

// parseOutTime interpreta timcodes HH:MM:SS.microseconds.
func parseOutTime(s string) (float64, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) > 1 {
		// mantém micros
		micro := parts[1]
		s = parts[0] + "." + micro[:min(len(micro), 3)]
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return 0, fmt.Errorf("formato out_time inválido: %s", s)
	}
	h, _ := strconv.ParseFloat(hms[0], 64)
	mint, _ := strconv.ParseFloat(hms[1], 64)
	sec, _ := strconv.ParseFloat(strings.Split(hms[2], ".")[0], 64)
	micro := 0.0
	if len(parts) > 1 {
		micro, _ = strconv.ParseFloat("0."+parts[1], 64)
	}
	return h*3600 + mint*60 + sec + micro, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ core.TranscoderEngine = (*Transcode)(nil)
