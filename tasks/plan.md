# Plano de Implementação: CodecAny v1.1

Este plano descreve o planejamento e a divisão de tarefas para a implementação das modificações da **Versão 1.1** do CodecAny, incluindo notificações de webhook, suporte declarativo para aceleração por hardware, relatório consolidado de economia no terminal e suporte a legendas externas.

## 📋 Visão Geral

O objetivo da v1.1 é estender o comportamento do CLI e do Core com novos recursos de conveniência, visibilidade e performance. A implementação será feita de forma incremental através de fatias verticais testadas localmente.

---

## 📐 Decisões de Arquitetura

1. **Aceleração de Hardware (`hwaccel`)**:
   - Adicionar os campos `hwaccel` (string) nos alvos de vídeo no [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go) e [types.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/types.go).
   - O adaptador FFmpeg lerá essa propriedade em [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) e injetará as flags `-hwaccel` correspondentes no comando CLI gerado.

2. **Relatório Consolidado de Economia**:
   - O banco de dados do [store.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go) manterá as somas. Faremos uma consulta agregadora de soma para todos os registros marcados como `COMPLETED`.
   - Ao encerrar amigavelmente o processo do terminal em [main.go](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go), esta métrica consolidada será desenhada de forma estilizada.

3. **Mapeamento de Legendas Externas**:
   - Em [probe.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/probe.go), faremos um scan de arquivos irmãos na mesma pasta com extensões `.srt`, `.vtt` e `.ass`.
   - O contêiner de destino sempre será idêntico ao de entrada (não haverá mudança de formato), facilitando o remux direto sem conversão de legendas.
   - Adicionaremos esses arquivos como inputs extras em [transcode.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) com instruções de mapeamento para o container de destino.

4. **Webhooks Locais de Notificação**:
   - Criação de uma rotina simples em Go que executa requisições POST HTTP em background, evitando o travamento do worker principal.
   - Adição de um mecanismo de retentativas automáticas (3 tentativas com backoff simples de 2s) em caso de falha de conexão ou HTTP status de erro.
   - Configurações declaradas globalmente no arquivo `rules.yaml` carregadas no [rules.go](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go).

---

## ⛓️ Grafo de Dependências

```
   [Task 1: HW Rules Model]             [Task 3: Savings DB Logic]          [Task 5: Subtitle Probe]
              │                                      │                                 │
   [Task 2: FFmpeg HW Accel]            [Task 4: CLI Summary Report]        [Task 6: Subtitle Remux]
              │                                      │                                 │
              └───────────────────┬──────────────────┘                                 │
                                  ▼                                                    │
                      [Task 7: Webhook Parsing & Client] <─────────────────────────────┘
                                  │
                      [Task 8: Engine Async Dispatch]
```

---

## 📝 Lista de Tarefas

### Fase 1: Aceleração de Hardware e Relatório de Economia

#### Tarefa 1: Modelagem e Parsing das Regras de HW-Accel
* **Descrição**: Adicionar campos de aceleração de hardware nas estruturas do TargetSpec e arquivo de configuração rules.yaml.
* **Critérios de Aceitação**:
  - `TargetSpec` estendido para aceitar `VideoHWAccel` (ex: `nvenc`, `qsv`, `vaapi`, `videotoolbox`).
  - Parsing de YAML e JSON mapeando corretamente o novo parâmetro.
* **Verificação**:
  - `go test ./pkg/core -run TestExampleRulesParse`
* **Dependências**: Nenhuma
* **Arquivos Prováveis**:
  - `pkg/core/types.go`
  - `pkg/core/rules.go`
  - `pkg/core/rules_test.go`
* **Escopo Estimado**: S

#### Tarefa 2: Mapeamento de Argumentos de HW-Accel no Adaptador FFmpeg
* **Descrição**: Traduzir o spec de hardware para parâmetros do FFmpeg (`-hwaccel` e codecs de codificação acelerada por hardware).
* **Critérios de Aceitação**:
  - Tradução correta de `hevc` + `nvenc` para as flags `-hwaccel cuda -c:v hevc_nvenc`.
  - Passar nos testes unitários e de integração de comandos de transcodificação.
* **Verificação**:
  - `go test -v ./pkg/adapters/ffmpeg -run TestBuildArgs`
* **Dependências**: Tarefa 1
* **Arquivos Prováveis**:
  - `pkg/adapters/ffmpeg/transcode.go`
  - `pkg/adapters/ffmpeg/transcode_test.go`
* **Escopo Estimado**: S

#### Tarefa 3: Lógica de Banco de Dados de Economia Acumulada
* **Descrição**: Criar agregação em SQL SQLite WAL para obter a soma de todas as economias de espaço obtidas no histórico.
* **Critérios de Aceitação**:
  - Função `GetTotalSavings() (SizeMetrics, error)` que agrupa estatísticas agregadas de todos os jobs `COMPLETED`.
* **Verificação**:
  - Adição de teste unitário validando a agregação com jobs mockados em banco.
* **Dependências**: Nenhuma
* **Arquivos Prováveis**:
  - `pkg/core/store.go`
* **Escopo Estimado**: S

#### Tarefa 4: Exibição do Relatório de Economia no CLI
* **Descrição**: Exibir o sumário consolidado de economia no console ao final da execução ou término por sinal.
* **Critérios de Aceitação**:
  - Imprimir estatísticas legíveis (em MB/GB e taxa %) no fechamento do CLI de forma estilizada.
* **Verificação**:
  - Compilação limpa e teste manual simulando um job concluído.
* **Dependências**: Tarefa 3
* **Arquivos Prováveis**:
  - `cmd/cli/main.go`
* **Escopo Estimado**: S

### Checkpoint: Hardware & Economia
- [ ] Compilação do CLI e Core passam sem erros.
- [ ] Testes unitários de hardware e store passam.
- [ ] CLI imprime sumário ao terminar execução.

---

### Fase 2: Legendas Externas e Webhooks

#### Tarefa 5: Descoberta de Legendas Irmãs no Directório (Probe)
* **Descrição**: Modificar a inspeção para identificar legendas externas (`.srt`, `.vtt`, `.ass`) com o mesmo nome do vídeo no diretório e adicioná-las aos metadados da mídia.
* **Critérios de Aceitação**:
  - Mapear caminhos de legendas encontradas em `MediaInfo`.
* **Verificação**:
  - `go test -v ./pkg/adapters/ffmpeg -run TestProbe`
* **Dependências**: Nenhuma
* **Arquivos Prováveis**:
  - `pkg/core/types.go`
  - `pkg/adapters/ffmpeg/probe.go`
* **Escopo Estimado**: S

#### Tarefa 6: Remuxing de Legendas no Adaptador FFmpeg
* **Descrição**: Alterar a montagem de argumentos para adicionar as legendas detectadas como inputs adicionais no FFmpeg aplicando o mapeamento de streams adequado.
* **Critérios de Aceitação**:
  - Geração de comando FFmpeg contendo `-i legenda.srt -map 0 -map 1 -c:s copy` ou similar no container final.
* **Verificação**:
  - Testes integrados com mocks de legendas em `pkg/adapters/ffmpeg/transcode_test.go`.
* **Dependências**: Tarefa 5
* **Arquivos Prováveis**:
  - `pkg/adapters/ffmpeg/transcode.go`
  - `pkg/adapters/ffmpeg/transcode_test.go`
* **Escopo Estimado**: M

#### Tarefa 7: Cliente HTTP de Webhook e Configuração Declarativa
* **Descrição**: Implementar parsing das regras do Webhook (`global.notifications.webhook_url` e `events`) e criar cliente HTTP para envio assíncrono.
* **Critérios de Aceitação**:
  - Sucesso no envio de POST HTTP com JSON do Job ao simular eventos.
* **Verificação**:
  - `go test -v ./pkg/core -run TestWebhook`
* **Dependências**: Nenhuma
* **Arquivos Prováveis**:
  - `pkg/core/rules.go`
  - `pkg/core/events.go`
  - `pkg/core/webhook.go` (Novo arquivo)
* **Escopo Estimado**: S

#### Tarefa 8: Despacho Assíncrono de Eventos de Webhook no Engine
* **Descrição**: Conectar o cliente de webhook aos callbacks de fim de Job no loop de execução do Engine.
* **Critérios de Aceitação**:
  - Disparo em background sem interferir na performance do processamento do worker.
* **Verificação**:
  - Teste de integração de fluxo completo validando que os webhooks são despachados sem bloquear o encerramento do Engine.
* **Dependências**: Tarefa 7
* **Arquivos Prováveis**:
  - `pkg/core/engine.go`
* **Escopo Estimado**: S

### Checkpoint: Legendas & Webhooks
- [ ] Todos os testes unitários e de integração do sistema passam.
- [ ] Envio assíncrono de webhook verificado.
- [ ] Legendas mapeadas e embutidas corretamente no container.

---

## ⚠️ Riscos e Mitigações

| Risco | Impacto | Mitigação |
| :--- | :--- | :--- |
| Encoders de HW-Accel indisponíveis no Host | Médio | Fallback automático para codificação em software se o FFmpeg reportar falha na inicialização do hwaccel. |
| Timeouts no Webhook travando Workers | Alto | Executar as chamadas HTTP em goroutines dedicadas e definir timeout rígido de 5 segundos no HTTP Client. |
| Incompatibilidade de container com legendas | Médio | Copiar a legenda apenas se o container de destino suportar legendas embutidas (ex: mkv/mp4). |
