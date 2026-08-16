# Proposta: Painel de Controle Web para o CodecAny

## Contexto

O CodecAny continua sendo, por decisão de produto, **uma aplicação primariamente CLI**: binário estático único, sem Docker, sem servidor web monolítico embutido no core. O `README.md` já reserva `/cmd/server` como diretório futuro ("wrapper HTTP/WebSocket/SSE") e o `docs/ROADMAP.md` lista, como próximos passos da Versão 2, exatamente um "Headless Web Server" com API REST + WebSocket/SSE e um "Dashboard SPA Front-end". Este documento destrava esses dois itens do roadmap com um plano concreto.

O problema que o painel resolve: hoje, centralizar configuração (regras YAML, diretórios monitorados, limites de hwaccel, webhook) e tomar decisões operacionais (aprovar/rejeitar jobs em staging, entender por que um arquivo não foi convertido, acompanhar progresso) exige editar arquivos à mão e rodar subcomandos administrativos (`-list-staged`, `-approve`, `-history` etc.) — funcional, mas sem visão consolidada. O painel não substitui a CLI (que continua sendo o modo de operação "sem cabeça"/scriptável); ele é uma camada opcional de observação e configuração sobre o mesmo `pkg/core`, inspirada funcionalmente no Tdarr (não visualmente).

**Fonte**: este plano foi produzido a partir de exploração direta do código (`pkg/core/engine.go`, `types.go`, `events.go`, `rules.go`, `watcher.go`, `store.go`, `cleanup.go`, `integrity.go`, `interfaces.go`, `webhook.go`, `cmd/cli/main.go` e os respectivos `*_test.go`) e refinado por um agente de design dedicado.

**Identidade visual**: aprovada em [`docs/design_painel_controle.md`](design_painel_controle.md) ("Signal Path" — paleta preto/roxo/cyano, tipografia Fira Sans/Hack, sistema de chips e medidores).

## Decisões de produto já fechadas

| Decisão | Escolha |
|---|---|
| Stack do frontend | **React + TypeScript + Vite** (SPA separada, build próprio, estáticos embutidos no binário Go via `go:embed`) |
| Escopo | **Single-host apenas** — sem conceito de "Nodes"/múltiplas máquinas do Tdarr |
| Autenticação | **Nenhuma** — bind padrão em `127.0.0.1`, assume rede confiável |
| Edição de regras | **Builder visual completo** (CRUD + reorder + enable/disable + testador de regras), não só um editor de YAML cru |

Real-time é via **SSE** (`GET /api/events`), não WebSocket: o fluxo de eventos do Engine (`chan JobEvent`) já é unidirecional (servidor→cliente), então SSE cobre 100% da necessidade com `EventSource` nativo do browser (zero dependência extra no frontend) e infraestrutura mais simples no servidor.

---

## 1. Arquitetura

### Novo binário `cmd/server`

Espelha `cmd/cli/main.go::buildEngine`: mesma construção de `core.NewStore` / `core.NewRulesEngine` / `core.NewEngine` com `ffmpeg.NewProber()` / `ffmpeg.NewVerifier()` / `ffmpeg.NewTranscode()`. **Nenhuma dependência nova de `pkg/core` → HTTP** — o server só importa `pkg/core` e `pkg/adapters/ffmpeg`, exatamente como `cmd/cli` já faz. O core continua comunicando-se só através das interfaces puras (`MediaProber`, `MediaVerifier`, `TranscoderEngine`).

Config própria (`cmd/server/config.go`): `ServerConfig{Addr, DBPath, RulesPath, Workers, LogDir, LogLevel, ScanInterval}`, flags espelhando os nomes já usados em `cmd/cli` (`-db`, `-rules`, `-workers`, `-log-dir`, `-log-level`, `-scan-interval`) mais `-addr` (default `127.0.0.1:8383`).

**Diretórios monitorados persistidos**: hoje só existem como flags `-dir` efêmeras do `cmd/cli`, nunca persistidas. Em vez de um segundo arquivo de estado (risco de divergir do `.db`), a proposta adiciona uma tabela `watched_dirs` ao `Store` já existente — mesma conexão SQLite WAL já testada, uma única fonte de verdade.

### Roteamento HTTP

`net/http.ServeMux` do Go 1.22+ (`go.mod` já está em `go 1.26.5`) é suficiente: patterns com método (`"GET /api/jobs/{id}"`) e `r.PathValue("id")` cobrem toda a API abaixo, sem justificar um router externo (`chi`/`gorilla/mux`) — coerente com a filosofia de dependências mínimas do projeto. Sem CORS: o SPA embutido serve da mesma origem que a API; em dev, o proxy do Vite mantém tudo na mesma origem do ponto de vista do browser (inclusive `EventSource`).

### SSE hub (fan-out sem alterar o contrato do Engine)

`Engine.Events() <-chan JobEvent` continua como hoje — canal único, um consumidor. Esse consumidor passa a viver inteiramente em `cmd/server/sse.go`, **sem nenhuma mudança de assinatura no `Engine`**:

- Uma goroutine única drena `eng.Events()` e publica em um `Broadcaster` (`map[chan JobEvent]struct{}` protegido por mutex).
- Fan-out **não bloqueante** por cliente (canal bufferizado pequeno; `select` com `default` descarta se o cliente estiver lento) — um consumidor SSE lento nunca aplica back-pressure no worker pool.
- Handler `GET /api/events` registra um canal, escreve frames `event: <Kind>\ndata: <JSON>\n\n`, desregistra ao `r.Context().Done()`.

### Estáticos do SPA via `go:embed`

Frontend em `/web` (novo diretório top-level), build próprio (`pnpm build`) com saída apontando para `cmd/server/webdist/` (precisa estar dentro do módulo Go para `go:embed` funcionar). `cmd/server/assets.go` declara `//go:embed all:webdist`, serve via `http.FileServerFS` na rota `/`, com fallback de SPA (path não-API inexistente cai para `index.html`, para o roteamento client-side funcionar). O binário final não depende de Node/pnpm em produção — só em build time (novo passo documentado no README: `corepack enable && cd web && pnpm install --frozen-lockfile && pnpm build` antes de `go build ./cmd/server`).

**Gerenciador de pacotes**: exclusivamente **pnpm** (via `corepack`, sem depender de instalação global) — `pnpm-lock.yaml` versionado, sem `package-lock.json`/`yarn.lock`. `web/package.json` deve fixar `"packageManager": "pnpm@<versão>"` para reprodutibilidade do build (CI e devs usam a mesma versão automaticamente via corepack).

---

## 2. Mudanças mínimas em `pkg/core` (aditivas, não quebram `cmd/cli` nem testes existentes)

| # | Mudança | Arquivo | Necessário no v1? |
|---|---|---|---|
| 1 | `Item.MarshalJSON`/`MarshalYAML` — **gap real**: `Item` (usado em `Match.Container`, `Match.Video.Codec`, `Match.Audio.Codec`) só tem `UnmarshalYAML`/`UnmarshalJSON` hoje. Sem marshal, serializar um `RuleFile` de volta produz `{"Values":["h264"]}` em vez de `"h264"`, quebrando API e o próprio `rules.yaml` editado pela UI | `pkg/core/rules.go` | **Sim** — bloqueia qualquer builder de regras |
| 2 | `Rule.Enabled` honrado em `Evaluate`/`ruleMatches`/`DescribeMiss`. Campo já existe no struct mas **não é lido em lugar nenhum hoje**. Convenção: `nil` = habilitada (retrocompat com `rules.yaml` existentes), `false` = desabilitada — **invertida** da convenção de `AutoApprove` (`boolVal` trata nil como false) | `pkg/core/rules.go` | **Sim** — requisito do builder |
| 3 | `RuleFile.Validate() error` (regras não vazias, `Action` válida, nomes não vazios, `min_saving_pct` em [0,100], `default_driver` não vazio); refatorar `NewRulesEngine` para reusar o mesmo `Validate()` | `pkg/core/rules.go` | **Sim** — exigido antes de persistir edições da UI |
| 4 | Hot-reload thread-safe **restrito ao casamento de regras** (`Rules` + `Global.Defaults`) via troca atômica de snapshot, exposto como `Engine.ReloadRules(path string) error`. **Fora do escopo do hot-reload v1**: `staging_dir`, `space_saving`, `hwaccel_limits`, `notifications` — capturados uma vez em `NewEngine` e usados para derivar `e.staging`/`e.integrity`/`e.hwSemaphores`/`e.webhook`; recalculá-los em runtime é arriscado (ex.: mudar `staging_dir` no meio de um job cujo `Cleanup` foi reconstruído deterministicamente a partir de `e.staging`). A UI deve avisar que essas mudanças exigem reiniciar o servidor | `pkg/core/rules.go`, `pkg/core/engine.go` | **Sim** (versão restrita) |
| 5 | `Watcher.RemoveDir(dir string) error` — refaz `filepath.Walk(dir)` no momento da remoção (não usar snapshot de `AddDir`, pois `scan()` adiciona subpastas criadas depois via `w.fsw.Add`), remove de `w.dirs`, purga `w.pending`/`w.scanned` sob esse caminho | `pkg/core/watcher.go` | **Sim** |
| 6 | `Store`: tabela `watched_dirs(path TEXT PRIMARY KEY, added_at TEXT)` + `AddWatchedDir`/`RemoveWatchedDir`/`ListWatchedDirs`; `Engine` expõe wrappers finos combinando Store+Watcher | `pkg/core/store.go`, `pkg/core/engine.go` | **Sim** |
| 7 | `Engine.HWAccelStatus() []HWAccelStatus{Vendor,Limit,InUse}` — introspecção de `e.hwSemaphores` (`Limit: cap(ch)`, `InUse: len(ch)`) | `pkg/core/engine.go` | **Sim** |
| 8 | `Store.CountByStatus() (map[JobStatus]int, error)` (`GROUP BY status`, usa `idx_jobs_status` já existente); wrapper no `Engine` | `pkg/core/store.go` | **Sim** |
| 9 | `Engine.GetJob(id string) (*Job, error)` — wrapper trivial de `Store.FindByID` (ainda não exposto pelo Engine) | `pkg/core/engine.go` | **Sim** |
| 10 | `Engine.RescanDirs() error` — reusa `ListWatchedDirs` + `core.DiscoverFiles` + `HandleDiscovered` (mesmo padrão de `RunOnce`) | `pkg/core/engine.go` | **Sim** |
| 11 | `pkg/core/healthcheck.go`: extrair o corpo de `cmd/cli/main.go::runHealthCheck` para `core.RunHealthCheck(dirs, files []string, verifier MediaVerifier) ([]HealthCheckResult, error)`; `cmd/cli` vira formatter fino sobre essa função | `pkg/core/healthcheck.go` (novo), `cmd/cli/main.go` | Fase E, não bloqueia v1 |
| 12 | `Store.SavingsByDay(since time.Time) ([]DailySavings, error)` | `pkg/core/store.go` | Opcional — v1 faz bucketing client-side sobre `GET /api/jobs` |
| 13 | `RulesEngine.File() RuleFile` — getter público do campo hoje privado `file` | `pkg/core/rules.go` | **Sim** |
| 14 | **Bug pré-existente, achado ao desenhar a coluna Original/Convertido de Fila e Histórico**: no ramo de rollback de `runJob` (`!keep`), `job.SizeMetrics = m` só atualiza a cópia em memória (usada no evento/webhook) — falta o `e.store.UpdateMetrics(job.ID, m)` que os ramos `AWAITING_APPROVAL`/`COMPLETED` já chamam antes de `UpdateStatus`. Resultado: todo job `ROLLED_BACK` persiste no banco com `original_size`/`converted_size`/`saved_bytes` zerados — `GET /api/jobs` nunca teria esses números pra exibir, mesmo já tendo sido calculados no momento do rollback. Corrigir adicionando a mesma chamada de `UpdateMetrics` antes do `UpdateStatus` no ramo `!keep` | `pkg/core/engine.go` (`runJob`, ramo `!keep`) | **Sim** — sem isso a coluna "Convertido"/"Economia" de linhas `ROLLED_BACK` fica sempre vazia |

`cleanup.go`, `integrity.go`, `signals.go`, `interfaces.go` **não precisam de nenhuma mudança** — `ApproveJob`/`RejectJob` já reconstroem `Cleanup` deterministicamente a partir de `job.Path`/`e.staging`/`job.Target.Container`, então os handlers de staging são só chamadas finas ao que já existe.

**Gap identificado, fora de escopo v1**: não há cancelamento por job — `Engine.CloseCancellation()` aborta *todos* os jobs em andamento via um canal compartilhado. Um botão "cancelar este job" na tela de Fila exigiria um mecanismo por-job inexistente hoje; não prometer essa funcionalidade no v1.

---

## 3. Superfície de API (REST + SSE)

Base `/api`, JSON (exceto `/api/events`), sem autenticação, bind padrão `127.0.0.1`.

**Geral / Dashboard**
- `GET /api/status` → `{version, uptime_s, addr, db_path, rules_path, workers}`
- `GET /api/dashboard/summary` → `{status_counts, total_savings, hwaccel}` (itens 7/8 + `GetTotalSavings`)
- `GET /api/events` (SSE) → frames por `JobEventKind`

**Fila/Jobs**
- `GET /api/jobs?status=&since=&until=` → `Engine.ListJobs(JobFilter{...})` (já existe)
- `GET /api/jobs/{id}` → item 9

**Aguardando Aprovação** (zero mudança de core, só handlers finos)
- `GET /api/staging`, `POST /api/staging/{id}/approve`, `POST /api/staging/{id}/reject`, `POST /api/staging/approve-all`, `POST /api/staging/reject-all`

**Diretórios Monitorados** (itens 5/6/10)
- `GET /api/dirs`, `POST /api/dirs {path}`, `DELETE /api/dirs?path=...`, `POST /api/dirs/rescan`
- `GET /api/fs/browse?path=` → `{path, parent, entries: [{name, path}]}` (só diretórios, ordenados). Suporte ao navegador de diretórios da UI (decisão de produto: preferido a um campo de texto puro). Implementado inteiramente em `cmd/server` (`os.ReadDir` + `filepath.Dir`, sem tocar `pkg/core`) — coerente com o modelo de confiança já assumido (sem auth, bind local): quem acessa a API já pode aprovar/rejeitar/editar regras, então navegar o filesystem não é uma classe de risco nova. Default `path=""` retorna a raiz (`/`).

**Regras** (itens 1/2/3/4/13)
- `GET /api/rules` → `RulesEngine.File()`
- `PUT /api/rules` (body `RuleFile`) → `Validate()` → serializa YAML → escreve com troca atômica (mesmo padrão de `cleanup.go::replaceAtomic`) → `Engine.ReloadRules(path)`. Reordenar é reenviar o array inteiro na nova ordem.
- `POST /api/rules/test` (`{path}` ou `{media_info}`) → `Probe` (se path) + `Evaluate` + `DescribeMiss` → `{outcome, target_spec, describe_miss}`

**Health Check** (item 11)
- `POST /api/health-check {dirs?, files?}` (default: diretórios monitorados) → síncrono no v1

**Histórico/Relatórios** — reaproveita `GET /api/jobs` (já suporta status/since/until) + `GET /api/dashboard/summary`, nenhum endpoint novo.

---

## 4. Telas (funcionalidade, inspirada no Tdarr)

- **Dashboard**: jobs ativos com progresso ao vivo (SSE), contagem por status, economia total, feed de eventos recentes, utilização de hwaccel por vendor.
- **Fila/Jobs**: tabela com filtro status/período, progresso ao vivo por linha, drawer de detalhe (MediaInfo de origem, `TargetSpec` aplicado, timestamps, erro). *Nota*: `Job` nunca re-sonda o arquivo final — "depois" na prática é o `TargetSpec` pedido (já validado por `Verify`), não uma nova inspeção do arquivo convertido; re-probe pós-commit fica para uma fase futura se for necessário.
- **Aguardando Aprovação**: tamanho original/convertido/economia/codec alvo, aprovar/rejeitar individual e em lote — backend 100% pronto. Ação individual não pede confirmação (clique deliberado numa tela feita pra isso); **"Aprovar todos"/"Rejeitar todos" pedem confirmação** (afetam N arquivos de uma vez, sem revisão individual, e a ação é irreversível — troca o arquivo original).
- **Diretórios Monitorados**: listar/adicionar/remover (persistidos), rescan manual. Adicionar diretório abre um **navegador de diretórios** (modal, via `GET /api/fs/browse`) em vez de campo de texto puro. `-dir` do `cmd/cli` continua efêmero e não toca a nova tabela — o painel se torna a fonte de verdade persistida a partir de agora, sem alterar o `cmd/cli` existente.
- **Regras**: builder completo (match + convert + auto_approve + enabled, reorder por drag) + configurações globais (space_saving, staging_dir, default_driver, hwaccel_limits, notifications) + testador de regras. **Escopo fechado**: só os campos que vêm do `rules.yaml` são editáveis aqui; parâmetros do *processo* do servidor (workers, bind address, paths de `-db`/`-rules`) aparecem em modo somente-leitura (via `GET /api/status`) — mudá-los exige editar flags e reiniciar de qualquer forma, então não há endpoint de escrita para eles no v1. UI deve avisar que mudanças em configs globais do `rules.yaml` (staging_dir/hwaccel_limits/notifications/default_driver) também exigem reiniciar o servidor — hot-reload é só do casamento de regras (ver item 4 da tabela de mudanças no core).
- **Health Check**: disparo sob demanda, lista de arquivos corrompidos.
- **Histórico/Relatórios**: tabela filtrável, sem endpoints novos.

---

## 5. Fases de implementação

1. **Fase A — Esqueleto do servidor** ✅ **concluída** (risco baixo, prova o pipeline): `cmd/server/{main,config,sse,router,assets}.go` + scaffold React/Vite em `/web`. Core: itens 7, 8, 9, **14** (o fix de `UpdateMetrics` no rollback — sem ele a tabela de Fila/Histórico exibe `ROLLED_BACK` sempre com Original/Convertido vazios). Endpoints: `/api/status`, `/api/dashboard/summary`, `/api/jobs(+/{id})`, `/api/events`. Entregável: `go run ./cmd/server` sobe, dashboard mostra contadores + progresso ao vivo — verificado de ponta a ponta (build real do frontend embutido, todos os endpoints, fallback de SPA, shutdown gracioso). Dois achados extra durante a implementação: (a) o padrão de sinal do `cmd/cli` (canal `os.Signal` bufferizado de tamanho 1 lido por duas goroutines) tem uma race latente — `cmd/server` usa `core.RunSignals` (context cancelado, broadcast seguro para N consumidores) em vez de copiar o padrão antigo; (b) o wrapper de `ResponseWriter` do middleware de log não propagava `Flush()`, quebrando SSE — corrigido com um passthrough explícito. `go build ./...`, `go vet ./...`, `gofmt -l .` e `go test ./...` limpos no repositório inteiro.
2. **Fase B — Staging/Aprovação** (risco muito baixo, valor alto): zero mudança de core, só handlers + UI. Maior valor/esforço de todas as fases — priorizar logo após A.
3. **Fase C — Diretórios Monitorados** (risco médio, primeira mudança de schema/Watcher): itens 5, 6, 10 + `GET /api/fs/browse` (server-only, sem mudança de core) para o navegador de diretórios da UI.
4. **Fase D — Regras/Config** (risco mais alto, maior valor de longo prazo):
   - D1 (backend isolado): itens 1, 2, 3, 4, testado com `-race` focado na troca atômica sob avaliação concorrente.
   - D2 (API + UI): `GET/PUT /api/rules`, `POST /api/rules/test`, builder completo.
5. **Fase E — Health Check + Histórico avançado** (risco baixo-médio): item 11 (+ refatoração de `cmd/cli`, validada por `main_test.go` continuando a passar sem mudança de comportamento); item 12 opcional.

**Racional da ordem**: A prova o pipeline de leitura/eventos com risco mínimo; B entrega o maior valor com o menor risco (backend já pronto) antes de qualquer mudança de schema; C introduz a primeira mudança de schema/Watcher isoladamente, antes de D atacar o problema mais arriscado (troca atômica de regras sob concorrência); E fica por último por reaproveitar padrões já estabelecidos em A-D.

---

## 6. Testes/verificação

**Go — `pkg/core`** (seguindo os padrões já existentes em `mocks_test.go`/`rules_test.go`/`watcher_test.go`/`store_test.go`/`engine_test.go`):
- `TestRuleEnabledSkipped`, `TestItemMarshalJSONRoundTrip`/`TestItemMarshalYAMLRoundTrip` (escalar único deve round-tripar como escalar, não array de 1), `TestRuleFileValidate`, `TestReloadRulesHotSwap` (reusando o padrão `controllableTranscoder`/`release` já usado em `TestEngineHWAccelLimitBlocksConcurrentTranscodes`, `engine_test.go:589`) rodando com `-race`.
- `TestWatcherRemoveDir` (AddDir→RemoveDir→arquivo criado depois não é mais promovido).
- `TestCountByStatus`, `TestWatchedDirsCRUD`, `TestHWAccelStatus`, `TestUnwatchDirPersists`.
- Fase E: `TestRunHealthCheck` com `mockVerifier`; garantir `cmd/cli/main_test.go` inalterado após a refatoração.

**Go — `cmd/server`** (novo, `package main` com fakes locais próprios — mesmo motivo de `fakeVerifier` em `cmd/cli/main_test.go`, que não pode importar tipos não-exportados de `pkg/core`):
- `httptest.NewServer` envolvendo o mux real, `Store` em tempdir + mocks locais.
- Cobrir: shape de `/api/dashboard/summary`; ciclo approve/reject completo contra fixture real; `/api/events` recebendo frame SSE bem formado; `PUT /api/rules` válido (persiste + hot-reload observável) e inválido (400, arquivo em disco inalterado); CRUD de `/api/dirs` contra subdiretórios reais em tempdir.
- Tudo em `go test ./...` (mocks, sem ffmpeg real), preservando a separação com `go test -tags integration ./...`.

**Verificação manual local**:
- `go run ./cmd/server -db /tmp/codecany-dev.db -rules rules.example.yaml -addr 127.0.0.1:8383`
- `cd web && pnpm dev` (proxy do Vite para `/api` e `/api/events`, só em dev)
- Build de produção: `cd web && pnpm install --frozen-lockfile && pnpm build` (saída em `cmd/server/webdist`) seguido de `go build ./cmd/server` — documentar no README/ROADMAP na Fase A.

---

## Arquivos críticos

- `pkg/core/engine.go`
- `pkg/core/rules.go`
- `pkg/core/store.go`
- `pkg/core/watcher.go`
- `cmd/cli/main.go` (referência de padrão, não modificado exceto na Fase E)
