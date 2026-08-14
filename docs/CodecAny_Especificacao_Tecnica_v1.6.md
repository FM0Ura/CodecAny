# CodecAny — Software Specification Document
> **Especificação Técnica e Arquitetural de Sistema Modular de Transcoding Autônomo**
> 
> * **Projeto:** CodecAny
> * **Tipo:** Submódulo / Standalone Headless
> * **Status:** Pronto para Desenvolvimento
> * **Versão:** 1.6.0 (Pragmatic Architecture & Simplicity Refinements)

---

## 1. Visão Geral do Projeto

O **CodecAny** é uma solução leve, desacoplada e autônoma para orquestração, inspeção e conversão de mídias (transcoding/automation). Inspirado no conceito do *Tdarr*, o CodecAny elimina todo o overhead associado a contêineres Docker, dependências de runtime pesadas e interfaces web monolíticas acopladas.

Projetado para operar como um binário estático nativo de alta performance (Go/Rust), ele pode rodar de forma independente via CLI/Daemon REST ou ser incorporado diretamente como um **submódulo reutilizável** dentro de um ecossistema de software maior.

---

## 2. Arquitetura Pragmática do Sistema

A arquitetura do CodecAny prioriza **simplicidade de manutenção, estabilidade e baixo footprint**. É orientada a eventos, baseada no padrão **Adapter/Strategy Pattern** para abstração de encoders, banco de dados embutido em arquivo único e comunicação por canais internos nativos.

```text
[ /pkg/core ] (Módulo Principal do Submódulo)
  ├── File Watcher (fsnotify + Timer de Debounce de tamanho)
  ├── SQLite Persistence (Modo WAL - arquivo único .db)
  ├── Queue Engine & Worker Manager (Comunicação via Channels / Observer Callbacks)
  └── Interfaces de Abstração
          ├── MediaProber (Interface de Inspeção)
          └── TranscoderEngine (Interface de Conversão)
                  │
                  └── [ /pkg/adapters/ffmpeg ] (Driver Padrão Inicial)

[ Executáveis do Projeto ]
  ├── /cmd/server (Wrapper fino para API REST / WebSockets / SSE)
  └── /cmd/cli    (Wrapper fino para execução em Terminal)
```

---

## 3. Requisitos Funcionais (RF)

| ID | Componente | Descrição Funcional |
| :--- | :--- | :--- |
| **RF01** | **Scanner & Watcher com Debounce** | Varrer diretórios configurados e monitorar arquivos em tempo real via `fsnotify`. Inclui timer de estabilização de tamanho (ex: 5s a 10s sem alteração em bytes) para evitar leituras de arquivos em gravação/download. |
| **RF02** | **Probe Engine (Pluggable)** | Invocar a interface `MediaProber` para extrair metadados técnicos (codecs de vídeo/áudio, bitrate, resolução, legendas, container) e armazenar cache no SQLite por hash/mtime. |
| **RF03** | **Rule Engine Pragmático** | Avaliar metadados contra um conjunto de regras declarativas (JSON/YAML) e extrair os alvos de conversão (`TargetCodec`, `TargetBitrate`, `Preset`). |
| **RF04** | **Job Runner & Transcoder Engine** | Gerenciar a fila no SQLite e invocar a interface `TranscoderEngine` no driver selecionado (padrão: FFmpeg), executando em staging e parseando progresso em tempo real. |
| **RF05** | **Integrity Check & Size Diff** | Validar integridade do arquivo e comparar tamanho original vs. convertido. Se o arquivo convertido for maior ou economizar menos do que o limite estipulado (ex: < 5%), aplicar rollback. |
| **RF06** | **API & Event Callbacks** | Expor eventos via canais nativos em memória (`chan JobEvent`) para o submódulo e expor rotas REST/WebSockets apenas na camada `/cmd/server`. |
| **RF07** | **Zero-Residue Clean Engine** | Garantir a remoção automática de todo e qualquer arquivo/diretório temporário criado durante o ciclo de vida do Job (staging, `.bak`, `.tmp`), mantendo apenas o output final validado e os logs. |

---

## 4. Decisões Arquiteturais Pragmáticas (Refinamentos v1.6.0)

1. **SQLite Exclusivo em Modo WAL (`journal_mode=WAL`):**
   * Substitui qualquer dependência de bancos *Key-Value* complexos.
   * O SQLite lida nativamente com concorrência leve, consultas relacionais de estatísticas e ordenação de prioridade da fila mantendo apenas **1 arquivo `.db`** no sistema.
2. **Eventos Nativos via Channels / Callback Directo:**
   * O pacote core (`/pkg/core`) não implementa barramentos Pub/Sub complexos. A comunicação de eventos (`OnJobStart`, `OnJobProgress`, `OnJobComplete`, `OnJobError`) é feita via canais/callbacks Go/Rust nativos.
   * A camada WebSocket e SSE é isolada exclusivamente dentro do executável de servidor (`/cmd/server`).
3. **Mecanismo de Lock por Debounce de Tamanho (`Stable-Size Timer`):**
   * O Watcher monitora modificações de arquivo e só promove o arquivo para `DISCOVERED` se o seu tamanho permanecer rigorosamente o mesmo durante um intervalo de segurança configurável.
4. **Isolamento da Tradução do Driver:**
   * A Engine de Regras não tenta criar uma "DSL universal de transcoding". Ela apenas entrega os parâmetros estruturados e o próprio Adapter do driver (ex: `/pkg/adapters/ffmpeg`) é responsável por traduzir essas estruturas para flags da sua CLI nativa.

---

## 5. Política de Limpeza e Gerenciamento de Resíduos (Zero-Residue Policy)

1. **Purga em Caso de Sucesso (`COMPLETED`):** Eliminação do arquivo `.bak` e purga da pasta de staging imediatamente após a validação do arquivo final.
2. **Purga em Caso de Erro ou Cancelamento (`FAILED` / `ROLLED_BACK`):** Interrupção do processo do worker, purga de arquivos `.tmp` e restauração do arquivo original via `.bak`.
3. **Purga de Boot (Orphan Recovery no Startup):** Varredura automática ao iniciar para deletar `.tmp` e restaurar `.bak` abandonados por crashes ou interrupções de energia.

---

## 6. Boas Práticas de Engenharia & CI/CD

* **Lock File & Dependências Strict:** Commit obrigatório do arquivo de lock (`go.sum` ou `Cargo.lock`).
* **Linting & Formatação:** Código deve passar sem avisos por linters estritos (`golangci-lint` ou `clippy`/`rustfmt`).
* **CI/CD Pipeline em 3 Stages:** Quality Gate (lint/tests), Cross-Compilation (Linux, macOS, Windows) e Release automatizado via SemVer.
* **Testes Unitários com Mocks de Interfaces:** Mocks das interfaces `MediaProber` e `TranscoderEngine` permitindo testar toda a fila e máquina de estados sem dependência de binários externos no ambiente de CI.

---

## 7. Métricas de Armazenamento e Observabilidade

* **Mapeamento por Job:** `original_size_bytes`, `converted_size_bytes`, `saved_bytes`, `compression_ratio_pct`.
* **Regra de Eficiência de Espaço:** Limite mínimo de economia configurável; efetua rollback se o ganho for irrelevante.
* **Structured Logging (JSON) & Métricas:** Logs padronizados (`job_id`, `file_path`, `driver_used`, `size_diff`) e métricas expostas via REST/SDK.

---

## 8. Requisitos Não-Funcionais (RNF) & Submódulo

| ID | Categoria | Especificação Técnica |
| :--- | :--- | :--- |
| **RNF01** | **Footprint & Memória** | Compilado como binário estático único sem dependência de runtimes externas. Consumo de RAM <= 50MB em idle. |
| **RNF02** | **Zero Overhead** | Invocação direta de executáveis nativos do Host. Proibido uso de camadas Docker/VM. |
| **RNF03** | **Persistência SQLite WAL** | Uso exclusivo do SQLite embutido em modo Write-Ahead Logging para arquivo único sem servidor. |
| **RNF04** | **Zero Residue Guarantee** | Garantia de remoção de qualquer arquivo temporário de trabalho após finalização/falha. |
| **RI01** | **Modularidade (Go/Rust)** | Core empacotado como biblioteca reutilizável; CLI e REST Server são apenas wrappers. |
| **RI02** | **Event Channels Nativos** | Comunicação de eventos via canais/callbacks em memória sem complexidade de barramentos Pub/Sub. |
| **RI03** | **Driver-Agnostic Design** | O orquestrador interage exclusivamente através das interfaces `MediaProber` e `TranscoderEngine`. |

---

## 9. Ciclo de Vida do Job e Máquina de Estados

| Estado | Descrição e Ações do Sistema |
| :--- | :--- |
| **`DISCOVERED`** | Arquivo identificado pelo Scanner após validação do Timer de Debounce de tamanho. |
| **`QUEUED`** | Inspeção via `MediaProber`. Regras validadas. Job persistido na fila do SQLite. |
| **`IN_PROGRESS`** | `TranscoderEngine` selecionado executa na pasta Staging. Output parseado em tempo real. |
| **`TESTING`** | Conversão finalizada. Validação de integridade do contêiner e ganho de tamanho. |
| **`FINALIZING`** | Se ganho de espaço >= limite: Substituição atômica via `.bak` + purga imediata do `.bak` e staging. Se ineficiente: Rollback + purga. |
| **`COMPLETED`** | Sucesso reportado. Métrica de espaço registrada. Evento `OnJobComplete` emitido via canal nativo. Zero resíduos em disco. |
| **`FAILED` / `ROLLED_BACK`** | Cancela worker, limpa staging e restaura o arquivo original. |

---

## 10. Diretrizes para a IA Dev (Instruções de Implementação)

1. **Persistência Simples (SQLite WAL):** Utilizar driver SQLite leve (ex: `modernc.org/sqlite` em Go sem dependência de CGO se possível) e inicializar sempre com `PRAGMA journal_mode=WAL;`.
2. **Strategy & Adapter Patterns:** Implementar o Core utilizando interfaces puras para `MediaProber` e `TranscoderEngine`. Isolar o FFmpeg e FFprobe na pasta `/pkg/adapters/ffmpeg`.
3. **Timer de Estabilização no Watcher:** Criar rotina de debounce no File Watcher que aguarda o tamanho do arquivo permanecer estável antes de enfileirar o Job.
4. **Mecanismo de Cleanup (Defer Pattern):** Utilizar blocos de limpeza garantidos (ex: `defer cleanup()` em Go ou RAII/Drop guards em Rust) no ciclo de vida de cada worker.
5. **Estrutura de Pacotes:**
   * `/pkg/core` (Orquestrador, Watcher com Debounce, Fila SQLite, Interfaces)
   * `/pkg/adapters/ffmpeg` (Driver FFmpeg/FFprobe padrão)
   * `/cmd/server` (Wrapper HTTP/WebSocket/SSE opcional)
   * `/cmd/cli` (Wrapper de linha de comando)
6. **Graceful Shutdown:** Tratar sinais do SO (`SIGINT`/`SIGTERM`) para abortar Jobs e purgar resíduos antes de encerrar.
