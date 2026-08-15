# Propostas de Implementação para a Versão 1.1

Para a **versão 1.1** (uma release incremental e focada em qualidade de vida e melhorias do Core/CLI sem mudar a arquitetura para um servidor web/REST), as seguintes melhorias trazem alto valor agregado com baixo custo de implementação:

---

## 📋 Resumo das Propostas Candidatas

| Proposta | Componente Impactado | Descrição |
| :--- | :--- | :--- |
| **1. Notificação via Webhook Local** | [events.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/events.go) & [engine.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/engine.go) | Dispara um POST JSON simples para uma URL configurada ao completar ou falhar um job. |
| **2. Aceleração de Hardware via Regras** | [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go) & [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) | Suporte declarativo a encoders de hardware (NVENC, VAAPI, etc.) diretamente no `TargetSpec`. |
| **3. Relatório Consolidado de Economia** | [store.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go) & [main.go](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go) | Exibe no console um sumário de bytes economizados e taxa média de compressão ao fim do processo. |
| **4. Incorporação de Legendas Externas** | [probe.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/probe.go) & [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) | Mapeamento automático de legendas `.srt`/`.vtt` de mesmo nome no diretório para remux final. |

---

## 🔍 Detalhamento Técnico das Propostas

### 1. Notificação via Webhook Local (Webhooks simples)
* **Objetivo**: Integrar com Discord, Slack ou servidores de notificação de forma imediata e sem precisar de um servidor web escutando no CodecAny.
* **Funcionamento**:
  - Adicionar a struct `global.notifications` no YAML/JSON avaliado por [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go), contendo `webhook_url` e `events` (lista de eventos a notificar, ex: `[completed, failed]`).
  - No loop de eventos em [events.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/events.go) ou em um listener associado ao `OnJobComplete`/`OnJobError` no [engine.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/engine.go), realizar uma requisição HTTP POST assíncrona contendo o payload estruturado do `Job` com os dados de compressão ou detalhes do erro.

### 2. Aceleração de Hardware Declarativa
* **Objetivo**: Aproveitar o poder de GPUs locais de forma simples.
* **Funcionamento**:
  - Expandir o [types.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/types.go) e o motor de regras [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go) para aceitar campos como `hwaccel` (ex: `cuda`, `vaapi`, `qsv`) e mapear codecs de hardware específicos (ex: `hevc_nvenc`, `h264_qsv`).
  - No construtor de argumentos de transcodificação de [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go), incluir as flags de hardware correspondentes antes dos inputs e na seleção de codec de vídeo.

### 3. Relatório Consolidado de Economia de Espaço
* **Objetivo**: Apresentar ao usuário o ganho de espaço acumulado.
* **Funcionamento**:
  - Criar um método no [store.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go) como `GetTotalSavings() (SizeMetrics, error)` que soma `original_size`, `converted_size` e `saved_bytes` de todos os jobs em estado `COMPLETED`.
  - Ao encerrar o CLI (no final de `RunOnce` ou ao receber um sinal de shutdown amigável no [main.go](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go)), imprimir um bloco estilizado no terminal consolidando:
    * Total de arquivos processados.
    * Espaço total consumido antes × depois.
    * Total de bytes e GB salvos.
    * Razão de compressão média.

### 4. Incorporação de Legendas Externas
* **Objetivo**: Não perder as faixas de legenda externas associadas ao vídeo durante o remux.
* **Funcionamento**:
  - Durante o `Probe` no [probe.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/probe.go), buscar por arquivos do tipo `.srt`, `.vtt` ou `.ass` que compartilham o mesmo nome base do arquivo de vídeo no diretório.
  - Ao invocar o [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go), mapear essas legendas como inputs adicionais do FFmpeg e aplicar `-c:s copy` ou `-c:s srt` para consolidá-las como faixas de legenda embutidas no contêiner de destino (ex: MKV).
