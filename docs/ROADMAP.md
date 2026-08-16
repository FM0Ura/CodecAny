# CodecAny — Roadmap de Desenvolvimento e Entregas (Versão 1)

Este documento apresenta o **Roadmap da Versão 1** do CodecAny. O sistema foi concebido como um submódulo modular e autônomo para **transcoding de mídia** em Go, sem dependência de containers Docker ou servidores web pesados.

Abaixo estão detalhados todos os componentes desenvolvidos, a arquitetura implementada, os resultados dos testes e o direcionamento para as próximas etapas do projeto.

---

## 🗺️ Visão Geral do Roadmap - Versão 1

A primeira versão foca em consolidar o **Core Engine** do sistema de transcodificação, a persistência e orquestração de jobs, e o driver padrão baseado em **FFmpeg / FFprobe**. 

```mermaid
graph TD
    A[Monitoramento: Watcher & Debounce] -->|Novos arquivos| B[Orquestrador: Engine]
    B -->|Inspeção de metadados| C[Probe & Validação de Regras]
    C -->|Fila priorizada| D[SQLite WAL Store]
    D -->|Execução em Staging| E[Worker Pool & Transcode Engine]
    E -->|Comparação e Integridade| F[Integrity Check & Rollback]
    F -->|Substituição Atômica| G[Zero-Residue Commit]
    F -->|Ganho insuficiente ou falha| H[Rollback & Limpeza]
```

---

## 🛠️ O Que Foi Concluído (Versão 1.0 - O Core Estável)

### 1. Núcleo de Orquestração (Core Engine)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Implementação do loop concorrente de trabalhadores (`workerLoop`) no [engine.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/engine.go).
  - Máquina de estados completa para os jobs (`DISCOVERED` ➔ `QUEUED` ➔ `IN_PROGRESS` ➔ `TESTING` ➔ `FINALIZING` ➔ `COMPLETED` / `FAILED` / `ROLLED_BACK`), conforme definido no [types.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/types.go).
  - Barramento de eventos em memória simplificado usando Go channels nativos (`chan JobEvent`) no [events.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/events.go).
  - Captura e tratamento de sinais do SO (`SIGINT` e `SIGTERM`) para cancelamento e purga imediata de jobs em andamento no [signals.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/signals.go).

### 2. Monitoramento Ativo com Debounce (Watcher)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Integração com `fsnotify` no [watcher.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/watcher.go) para monitoramento em tempo real.
  - Mecanismo de debounce baseado em tamanho estável (`stable-size timer`) para evitar ler arquivos em processo de download/cópia.
  - Varredura periódica de fallback (`scan`) para detectar arquivos pré-existentes ou renomeados externamente.

### 3. Fila Persistente e Banco de Dados (Store)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Integração com SQLite sem CGO (`modernc.org/sqlite`) no [store.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go).
  - Inicialização do banco em modo **Write-Ahead Logging (WAL)** para excelente concorrência em arquivo único.
  - Ordenação por prioridade do job com FIFO para desempate de criação.

### 4. Avaliação de Regras Declarativas (Rules Engine)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Motor de regras declarativo baseado em JSON ou YAML no [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go).
  - Lógica *first-match wins* (primeira regra que atende executa).
  - Suporte a verificação de codec escalar ou em listas de OR (ex: `[aac, ac3]`).
  - Configuração de ações (`convert` ou `skip`) e regras de ignorar (`ignore`) por pasta, sufixo de arquivo ou tamanho mínimo.
  - Preservação ou reencodificação de faixas de áudio sem impedir a conversão principal do vídeo.

### 5. Mecanismo de Limpeza e Transacionalidade (Cleanup)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Estrutura transacional em [cleanup.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/cleanup.go) baseada no padrão de limpeza em escopo (`defer`).
  - Criação de backup `.bak` local para permitir substituição atômica no mesmo dispositivo de bloco.
  - Mecanismo inteligente de fallback (`replaceAtomic`) para copiar o arquivo antes de mover quando o diretório de staging está em outro filesystem/dispositivo (evitando falha por `EXDEV`).
  - Varredura de boot (`bootRecover`) para detecção e limpeza automática de resíduos órfãos resultantes de crashes anteriores do sistema.

### 6. Verificação de Eficiência e Integridade (Integrity)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Validação matemática de ganho de espaço no [integrity.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/integrity.go).
  - Configuração ajustável para ganho mínimo (padrão de 15%). Executa rollback se o arquivo final for maior ou economizar menos do que o estipulado.
  - Emissão de metadados refinados de armazenamento (`original_size_bytes`, `converted_size_bytes`, `saved_bytes`).

### 7. Driver Adaptador FFmpeg e FFprobe
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Implementação das interfaces [interfaces.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/interfaces.go) sob o driver padrão `ffmpeg`.
  - Inspeção via ffprobe no [probe.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/probe.go) com cache.
  - Tradução do `TargetSpec` em comandos reais de FFmpeg no [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go).
  - Verificação de decodificação completa do arquivo transcodificado no [verify.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/verify.go) antes do swap final.
  - Parseamento contínuo da saída `-progress pipe:1` para reportar percentual de progresso em tempo real.

### 8. CLI e Logging Estruturado
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - Ponto de entrada CLI completo no [main.go](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go) suportando flags flexíveis (`-db`, `-rules`, `-workers`, `-dir`, `-file`, `-json`, `-scan-interval`).
  - Suporte a múltiplos diretórios de monitoramento simultâneos e processamento direto "one-shot" de arquivos individuais.
  - Logger estruturado no [logger.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/logger/logger.go) com rotação automática via `lumberjack.v2` e opção de saída console em texto plano (desenvolvimento) ou JSON (produção).
  - Exibição de progresso em barra de carregamento interativa em terminais suportados.
---

## 🚀 Novidades e Melhorias da Versão 1.1 (Aceleração de HW, Legendas e Webhooks)
* **Status**: 100% Concluído e testado.
* **Detalhes**:
  - **Aceleração por Hardware (`hwaccel`)**: Adicionada a propriedade `VideoHWAccel` no [types.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/types.go) e motor de regras em [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go). O adaptador FFmpeg em [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) agora mapeia e injeta os comandos `-hwaccel` correspondentes antes do input de vídeo.
  - **Relatório de Economia Consolidada**: Consulta SQL agregada implementada em [store.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go) com testes unitários em [store_test.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store_test.go). O CLI em [main.go](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go) agora imprime um relatório no console ao encerrar o processamento.
  - **Mapeamento de Legendas Externas**: O prober em [probe.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/probe.go) detecta automaticamente arquivos `.srt`/`.vtt`/`.ass` irmãos e o transcoder em [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) realiza o remuxing desses streams de legenda diretamente para o arquivo final.
  - **Webhooks de Notificação com Retentativas**: Cliente HTTP implementado em [webhook.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/webhook.go) (testado em [webhook_test.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/webhook_test.go)) que dispara payloads JSON de forma assíncrona sobre o status dos jobs, contando com **3 tentativas de reenvio em caso de falha**.

---

## 🔧 Melhorias Adicionais do Core (Versão 1.2)
* **Status**: 100% Concluído e testado. Ver [`docs/propostas_v1.2_adicional.md`](propostas_v1.2_adicional.md) para o detalhamento técnico completo de cada proposta.
* **Detalhes**:
  - **Regras por Resolução/Bitrate**: `Match.Video` ganhou `min_height`/`max_height`/`min_bitrate_kbps` em [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go); `ConvertSpec.Video.MaxHeight` aplica downscale opcional via `-vf scale` em [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) — primeiro filtro de vídeo do projeto, abrindo caminho para filtros futuros (crop, deinterlace, tonemap HDR→SDR).
  - **Remux-only**: regras com `convert.video.codec: copy` trocam só o container, sem recodificar vídeo — isenta esses jobs da política de economia de espaço (não faz sentido medir "ganho" de uma cópia).
  - **Limite de Concorrência por GPU/Hwaccel**: semáforo por vendor (`global.hwaccel_limits`) em [engine.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/engine.go) evita estourar sessões simultâneas de encoders físicos (ex.: NVENC), independente do pool de `-workers`.
  - **Modo Health Check Standalone**: `-health-check` varre `-dir`/`-file` em busca de arquivos corrompidos via decodificação completa (mesma verificação usada antes de cada troca de arquivo), sem transcodificar nada — extraído depois para `core.RunHealthCheck` (ver seção do Painel de Controle abaixo) para ser reutilizável fora da CLI.
  - **Histórico/Relatório Filtrável**: `-history` (com `-status`/`-since`) consulta jobs por status/período no [store.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go), complementando o relatório agregado de economia já existente.
  - **Fallback de progresso por contagem de frames**: em containers com múltiplos streams de saída (vídeo+capa+múltiplas faixas de áudio+legendas), o ffmpeg pode reportar `out_time`/`out_time_ms` como `N/A` durante toda a conversão — `readProgress` em [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) agora usa `frame/totalFrames` como reserva nesse caso, usando o `MediaInfo.FrameRate` extraído do probe.

---

## 🎛️ Painel de Controle Web (`cmd/server` + `/web`) — Versão 1.3
* **Status**: 100% Concluído e testado — as 5 fases planejadas em [`docs/propostas_painel_controle.md`](propostas_painel_controle.md) foram implementadas, mergeadas e verificadas de ponta a ponta. Identidade visual completa ("Signal Path" — paleta preto/roxo/cyano) em [`docs/design_painel_controle.md`](design_painel_controle.md).
* **Arquitetura**: novo binário opcional `cmd/server` (servidor REST + SSE embarcado, `net/http` puro sem router externo) servindo um SPA React + TypeScript + Vite (`/web`, gerenciado exclusivamente com **pnpm**) via `go:embed` — consome `pkg/core` pelas mesmas interfaces puras que `cmd/cli`, sem nenhuma mudança de arquitetura do core. Zero autenticação, bind padrão em `127.0.0.1` (rede confiável); acesso remoto documentado via Tailscale no [README](../README.md#acesso-remoto-via-tailscale).
* **Fases entregues**:
  - ✅ **Fase A — Esqueleto do servidor**: endpoints somente-leitura (`/api/status`, `/api/dashboard/summary`, `/api/jobs`, `/api/events` via SSE) e Dashboard funcional (contadores por status, economia acumulada, utilização de hwaccel, progresso ao vivo).
  - ✅ **Fase B — Staging/Aprovação**: telas Fila e Aguardando Aprovação; aprovar/rejeitar individual (sem confirmação) ou em lote (com confirmação, por serem irreversíveis) — zero mudança de core, o backend (`ApproveJob`/`RejectJob`/`ApproveAll`/`RejectAll`/`ListStaged`) já estava pronto.
  - ✅ **Fase C — Diretórios Monitorados**: tabela `watched_dirs` persistida no Store (antes só existia como flags `-dir` efêmeras), `Watcher.RemoveDir`, navegador de filesystem (`GET /api/fs/browse`) para a UI de "adicionar diretório".
  - ✅ **Fase D — Regras/Config**: builder visual completo (match + convert + auto_approve + enabled, reorder), testador de regras, hot-reload atômico do casamento de regras (`Engine.ReloadRules`, sem reiniciar o servidor — provado seguro sob concorrência com `-race`). Exigiu `Item.MarshalJSON`/`MarshalYAML` (gap real: sem isso um valor escalar serializava como array de 1 elemento) e `Rule.Enabled` finalmente sendo honrado na avaliação.
  - ✅ **Fase E — Health Check + Histórico**: `core.RunHealthCheck` extraído do `cmd/cli` (reusado por ambos, CLI virou formatter fino); tela de Histórico com gráfico de economia por dia.
* **Achados de segurança corrigidos ao longo da implementação** (todos com teste de regressão):
  - Job `ROLLED_BACK` não persistia `SizeMetrics` no banco (só a cópia em memória) — achado desenhando as colunas Original/Convertido do painel.
  - `JobEvent.Metrics` era struct por valor com `omitempty` — que não tem efeito em campos struct — então todo evento SSE (inclusive `OnJobProgress`) carregava métricas zeradas; o Dashboard sempre mostrava "0 B → 0 B" durante o progresso. Virou ponteiro; `OnJobComplete` passou a carregar as métricas reais também (antes só `OnJobAwaitingApproval` carregava).
  - `POST /api/health-check` com corpo vazio só enxergava diretórios passados por `-dir` no boot, nunca os adicionados só pela tela Diretórios — corrigido para consultar `Engine.ListWatchedDirs()` a cada request.
  - **Recuperação de interrupção dura** (`kill -9`, queda de energia): `Cleanup.Commit()` troca o arquivo em duas chamadas `os.Rename` separadas; se o processo morre entre as duas, o arquivo esperado sumia do disco sem nenhuma recuperação automática no boot (`bootRecover` só cobria `IN_PROGRESS`). `Engine.recoverStuckFinalization` agora repara jobs presos em `TESTING`/`FINALIZING` no boot, restaurando o backup com segurança e liberando o path para reprocessamento — o arquivo original nunca é perdido. Um `Ctrl+C`/`SIGTERM` único já era seguro (shutdown gracioso espera a etapa em curso terminar).
* **Build/dev**: `make gui-build`/`make gui-run`/`make gui-dev` (Makefile restilizado com a paleta "Signal Path"); equivalente manual documentado no README.

---

## 📊 Cobertura de Testes e Qualidade

O projeto conta com uma cobertura completa dividida em:
1. **Testes Unitários**: Validação da lógica da máquina de estados, regras de transição, detecção de debounce e integridade utilizando Mocks das interfaces em [mocks_test.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/mocks_test.go). Inclui casos de concorrência rodados com `-race` (hot-reload de regras sob avaliação concorrente, boot recovery de jobs interrompidos).
2. **Testes de Integração**: Execução real chamando os wrappers de CLI e ffmpeg/ffprobe locais no [integration_test.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/integration_test.go).
3. **Testes do painel de controle** (`cmd/server`, `package main` com fakes locais próprios): `httptest.NewServer` sobre o mux real, cobrindo cada endpoint REST, ciclo de vida completo de aprovação/rejeição, frames SSE bem formados, CRUD de diretórios e regras (persiste + hot-reload observável).

Todos os testes encontram-se passando com sucesso, garantindo estabilidade no deploy contínuo (CI/CD):
* **Sucesso na compilação estática**: `make build`/`make gui-build` geram os binários portáteis `./codecany` e `./codecany-server` (frontend embutido via `go:embed`).
* **Zero avisos de linting**: Alinhado com o [Makefile](file:///home/fmoura/Documents/Applications/CodecAny/Makefile) e `.golangci.yml`.

---

## 🔮 Próximos Passos (Versão 2 e Futuro)

Com a fundação estável e testada da **Versão 1** — incluindo o core estendido da Versão 1.2 e o painel de controle web completo da Versão 1.3 —, os próximos passos estão direcionados à expansão de conectividade e otimizações avançadas:

### 🚀 Marcos Planejados:
1. **Otimizações Automáticas de Hardware (GPU Transcoding)**:
   - Detecção automatizada de aceleração no host (Nvidia NVENC, Intel QuickSync, Apple VideoToolbox).
   - Inserção de presets específicos nos adapters para transcodificação acelerada por hardware de altíssima performance.
2. **Notificações Integradas (Notifiers)**:
   - Suporte a triggers de Webhooks ao concluir jobs com sucesso ou falha (integração com Discord, Slack, Telegram).
