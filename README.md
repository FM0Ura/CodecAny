# CodecAny

Sistema modular autônomo de **transcoding de mídia** para Go. Inspirado no *Tdarr*, é um binário estático nativo de baixo footprint que orquestra e converte vídeos **sem Docker**, **sem servidor web monolítico** e **sem runtime pesado**.

## Como funciona

Diretórios monitorados (via `fsnotify`) → arquivo é **inspecionado** (`ffprobe`) → metadados avaliados contra **regras declarativas** (YAML/JSON) → se casar, é **enfileirado** no SQLite (modo WAL) → `ffmpeg` converte em *staging* → validado por **economia de espaço** e **integridade** → troca atômica com **rollback automático** se o ganho for < 15% → **zero resíduos** em disco.

```
DISCOVERED → QUEUED → IN_PROGRESS → TESTING → FINALIZING → COMPLETED
                                              ↘ ROLLED_BACK / FAILED
```

## Requisitos

- Go 1.21+ (ou apenas o binário compilado)
- `ffmpeg` e `ffprobe` no `PATH` (para o driver padrão)

## Uso (CLI)

```bash
go build -o codecany ./cmd/cli

./codecany -db codecany.db -rules examples/rules.yaml -worker 1 -dir /mnt/media
# múltiplos diretórios
./codecany -dir /mnt/filmes -dir /mnt/series -dir /mnt/tv
# log estruturado JSON
./codecany -json -dir /mnt/media
```

### Flags

| Flag | Default | Descrição |
|---|---|---|
| `-db` | `codecany.db` | arquivo SQLite (WAL) |
| `-rules` | `rules.yaml` | arquivo de regras YAML/JSON |
| `-workers` | `1` | número de workers |
| `-dir` | — | diretório a monitorar (repetível) |
| `-json` | `false` | log estruturado JSON |

`SIGINT`/`SIGTERM` abortam jobs e purgam resíduos com segurança.

## Regras (`rules.yaml`)

Veja [`examples/rules.yaml`](examples/rules.yaml). Resumo:

- **First-match wins** — a primeira regra que casar vence.
- **`match`** por igualdade escalar (`codec: h264`) ou lista OR (`codec: [aac, ac3]`).
- **`action: skip`** define regras de "não converter".
- **`global.space_saving.min_saving_pct`** — limite de rollback (default **15%**).

## Arquitetura

```
/pkg/core              → orquestrador, watcher com debounce, fila SQLite, regras
/pkg/adapters/ffmpeg   → driver padrão (ffprobe + ffmpeg)
/cmd/cli               → wrapper de linha de comando
/cmd/server            → (futuro) wrapper HTTP/WebSocket/SSE
```

Interfaces puras (`MediaProber`, `TranscoderEngine`) permitem drivers alternativos sem tocar no core.

## Desenvolvimento

```bash
go test ./... -count=1                  # unitários (mocks, sem ffmpeg)
go test -tags integration ./...          # integração (requer ffmpeg)
gofmt -l .
go vet ./...
```

CI (GitHub Actions): quality gate → cross-compile (linux/macos/windows) → release SemVer.

## Licença

MIT