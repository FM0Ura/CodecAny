# Propostas Adicionais para a Versão 1.2: O que mais trazer do Tdarr

Este documento complementa o [`propostas_v1.2.md`](propostas_v1.2.md) (Staging e Aprovação de Transcodificação, que continua sendo a proposta principal da release). Aqui são levantadas **outras funcionalidades do Tdarr** ainda ausentes no CodecAny, avaliadas quanto a custo de implementação e aderência ao princípio do projeto de **"sem overhead e sem over-engineering"** — binário estático único, sem Docker, sem servidor web pesado no core.

O levantamento partiu de uma auditoria do código atual (`pkg/core` e `pkg/adapters/ffmpeg`) para garantir que nada aqui proposto já existe. As 5 propostas abaixo foram priorizadas por serem de baixo custo, reaproveitarem interfaces/estruturas já existentes e trazerem paridade com os casos de uso mais comuns do Tdarr.

---

## 📋 Resumo das Propostas

| Proposta | Componente Impactado | Descrição |
| :--- | :--- | :--- |
| **1. Regras por Resolução/Bitrate** | [`rules.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go) & [`transcode.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) | Match de regra por altura/largura/bitrate de vídeo (já extraídos, não usados) + downscale opcional via filtro `-vf scale`. |
| **2. Remux-only (`video.codec: copy`)** | [`rules.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/rules.go) & [`transcode.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/transcode.go) | Permite regras que só trocam o container (ex: `.avi` → `.mkv`) sem recodificar vídeo. |
| **3. Limite de Concorrência por GPU/Hwaccel** | [`engine.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/engine.go) | Semáforo por vendor de hardware para não estourar sessões simultâneas de encoders físicos (ex: NVENC). |
| **4. Modo Health Check Standalone** | [`verify.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/adapters/ffmpeg/verify.go), [`watcher.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/watcher.go) & [`main.go`](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go) | Varredura da biblioteca só para detectar arquivos corrompidos, sem transcodificar. |
| **5. Histórico/Relatório Filtrável de Jobs** | [`store.go`](file:///home/fmoura/Documents/Applications/CodecAny/pkg/core/store.go) & [`main.go`](file:///home/fmoura/Documents/Applications/CodecAny/cmd/cli/main.go) | Consulta de jobs por status/período, complementando o relatório agregado de economia já existente. |

---

## 🔍 Detalhamento Técnico das Propostas

### 1. Regras por Resolução/Bitrate

* **Estado atual**: `MediaInfo.Width`, `Height` e `VideoBitrate` já são extraídos pelo prober (`probe.go`), mas `ruleMatches` em `rules.go` só avalia `container` e `video.codec` — resolução e bitrate não são usados como critério de match.
* **Objetivo**: evitar reprocessar vídeos que já estão em resolução/bitrate eficientes, e permitir *downscale* declarativo de conteúdo 4K para 1080p quando o ganho de espaço compensar.
* **Funcionamento proposto**:
  - Estender `Match` (`rules.go`) com `video.min_height`, `video.max_height` e `video.min_bitrate_kbps` — comparações numéricas (`>=`/`<=`), diferente do match escalar/lista OR usado hoje para `codec`.
  - Estender `ConvertSpec.Video` (`types.go`) com `max_height` opcional. Quando definido e a altura de origem for maior, o transcoder aplica um filtro de downscale.
  - Em `transcode.go`, traduzir `max_height` para `-vf scale=-2:<max_height>` (mantém aspect ratio, força largura par). **Este é o primeiro filtro de vídeo do projeto** — hoje `transcode.go` não implementa nenhuma flag `-vf`/`-filter:v`. A introdução desse mecanismo abre caminho para filtros futuros (crop, deinterlace, tonemap HDR→SDR) sem exigir que sejam implementados agora.
* **Exemplo de YAML**:
  ```yaml
  rules:
    - name: "Downscale 4K para 1080p em H265"
      match:
        video:
          min_height: 1440
      convert:
        video:
          codec: h265
          max_height: 1080
  ```

---

### 2. Remux-only (`video.codec: copy`)

* **Estado atual**: `TargetSpec.VideoCodec` sempre passa pela lógica de presets/CRF/hwaccel de `resolveHWAccelEncoder` e afins (`transcode.go`). Não existe um caminho que emita `-c:v copy`.
* **Objetivo**: cobrir o caso de uso clássico do Tdarr de **remuxar sem recodificar** — por exemplo, trocar um `.avi` legado por `.mkv` só para ganhar compatibilidade de player/legendas, preservando 100% da qualidade original e sem gastar CPU/GPU com um re-encode desnecessário.
* **Funcionamento proposto**:
  - Tratar `video.codec: copy` como valor especial em `mergeSpec` (`rules.go`) e no construtor de argumentos (`transcode.go`), pulando toda a resolução de preset/CRF/hwaccel e emitindo diretamente `-c:v copy`.
  - Interação com áudio: já é possível hoje deixar `audio.codec` indefinido (o transcoder omite `-c:a` e deixa o ffmpeg decidir) ou usar `copy` explicitamente — nenhuma mudança necessária nesse lado.
* **⚠️ Ponto em aberto**: remux raramente gera economia de espaço (às vezes o arquivo cresce por causa do overhead de container). A política atual de rollback (`IntegrityCheck.MinSavingPct`, default 15%) reverteria **toda** conversão remux-only. Como hoje esse limite só existe em nível **global** (`global.space_saving.min_saving_pct`), a implementação precisa decidir entre:
  - (a) adicionar um override de `min_saving_pct` por regra, ou
  - (b) tratar `video.codec: copy` como isento automático da checagem de economia (a validação de integridade via `Verify()` continua se aplicando).
  Recomenda-se a opção (b) por ser mais simples e coerente com a intenção explícita da regra.

---

### 3. Limite de Concorrência por GPU/Hwaccel

* **Estado atual**: `Engine.workers` (`engine.go`) é um único inteiro global — todas as goroutines de worker competem pela mesma fila, sem diferenciação por driver/hwaccel. Os encoders de hardware já suportados pelo adapter ffmpeg são nvenc/cuda, vaapi, qsv e videotoolbox (`transcode.go:106-114`).
* **Objetivo**: evitar falhas de transcodificação por estouro de sessões simultâneas de hardware — placas consumer (ex: GeForce) tipicamente limitam sessões NVENC simultâneas a 2-3, independentemente de quantos workers de CPU o CodecAny tenha configurado.
* **Funcionamento proposto**:
  - Novo bloco `global.hwaccel_limits` no YAML, ex: `{nvenc: 2, vaapi: 1}`.
  - Implementar como semáforos (`chan struct{}` com buffer N) por vendor, criados na inicialização do `Engine`. Antes de `runJob` invocar `TranscoderEngine.Transcode`, se `Target.VideoHWAccel` corresponder a uma chave configurada, adquire um slot do semáforo correspondente (bloqueando se todos ocupados) e libera ao final do transcode.
  - Regras sem `hwaccel` configurado continuam usando apenas o pool global de `-workers`, sem nenhuma mudança de comportamento.

---

### 4. Modo Health Check Standalone

* **Estado atual**: `MediaVerifier.Verify(path)` (`verify.go`) já decodifica o arquivo inteiro sem gerar saída (`ffmpeg -v error -i path -f null -`) e reporta qualquer erro de decodificação, mas hoje só é chamado dentro do ciclo de vida completo do job (estado `TESTING`, após um transcode) — nunca de forma isolada.
* **Objetivo**: espelhar o "Health Check" independente do Tdarr — permitir varrer uma biblioteca inteira só para detectar arquivos corrompidos, **sem** enfileirar, transcodificar ou tocar em nenhum arquivo original.
* **Funcionamento proposto**:
  - Nova flag de CLI `-health-check` em `main.go`. Reaproveita a listagem de arquivos já feita pelo `Watcher`/`scan()` (mesmas extensões suportadas e mesma lógica de descoberta de diretórios via `-dir`), mas em vez de promover cada arquivo a `DISCOVERED`/enfileirar, chama diretamente `Verify()` e segue para o próximo.
  - Saída no console (e opcionalmente `-json`) listando arquivos corrompidos, ex: `[CORRUPTED] /mnt/media/x.mkv: erro de decodificação`.
  - Por ser essencialmente reuso de uma interface pura já existente (`MediaVerifier`), o custo de implementação é baixo — não introduz nenhum novo driver nem nova dependência externa.
* **Fora de escopo**: tentativa automática de reparo/remux de arquivos corrompidos identificados — proposta apenas de **detecção**, não de correção; correção fica para uma versão futura.

---

### 5. Histórico/Relatório Filtrável de Jobs

* **Estado atual**: `store.go` só expõe `GetTotalSavings()`, que agrega **todos** os jobs com status `COMPLETED` desde sempre — sem filtro por status, período ou pasta.
* **Objetivo**: permitir consultas pontuais como "quais jobs falharam nas últimas 24h" ou "quais foram revertidos por economia insuficiente", úteis tanto para debug manual quanto para operação em background (complementando, sem duplicar, o `-list-staged` já proposto em `propostas_v1.2.md` para jobs `AWAITING_APPROVAL`).
* **Funcionamento proposto**:
  - Novo método `ListJobs(filter JobFilter) ([]Job, error)` em `store.go`, reaproveitando o padrão de query já usado em `NextPendingJob()`. `JobFilter` com campos opcionais: `Status`, `Since`, `Until` (baseados em `created_at`/`finished_at`).
  - Novo subcomando de CLI `-history`, com flags `-status=FAILED` e `-since=24h` (parse de duração relativa), formatando a saída em tabela no mesmo estilo do `-list-staged`.

---

## 🧭 Consideradas e Adiadas

Duas funcionalidades adicionais do Tdarr foram levantadas e discutidas, mas **não** entraram nesta rodada de priorização — registradas aqui para retomada futura:

* **Filtro de faixas de áudio/legenda por idioma**: hoje o transcoder mapeia todas as faixas de áudio (`-map 0:a`) e legenda em bloco, sem seleção por idioma (o probe também não extrai `tags.language` de nenhum stream). É provavelmente a lacuna de maior impacto prático em economia de espaço real (remover faixas de comentário/dublagens indesejadas é um dos usos mais comuns de plugins Tdarr), mas exige mudanças tanto no probe (extração de idioma por stream) quanto no transcoder (filtragem por idioma/índice) — maior escopo que as 5 propostas acima.
* **Janela de silêncio agendada (quiet hours)**: pausar o início de novos jobs em horários configuráveis (ex: não competir com streaming de Plex às 19h-23h). Simples de implementar (checagem de horário antes de `NextPendingJob()`), mas foi deprioritizada por não resolver nenhuma dor concreta relatada até agora.

---

## ⚠️ Riscos e Mitigações

* **Remux-only e rollback por economia insuficiente** (Proposta 2):
  - *Risco*: se `video.codec: copy` não for isento da checagem de `min_saving_pct`, toda regra de remux-only sofreria rollback automático, tornando a feature inútil na prática.
  - *Mitigação*: isentar automaticamente jobs com `video.codec: copy` da validação de `min_saving_pct` (a checagem de integridade via `Verify()` continua obrigatória).

* **Semáforos de hwaccel mal configurados** (Proposta 3):
  - *Risco*: se o vendor resolvido em `transcode.go` (ex: alias `nvenc`→`cuda`) não corresponder exatamente à chave usada em `global.hwaccel_limits`, o limite simplesmente não se aplica (silenciosamente), levando ao mesmo problema de estouro de sessões que a feature deveria prevenir.
  - *Mitigação*: normalizar a chave de vendor usando a mesma função de resolução já usada pelo adapter (`resolveHWAccelEncoder`), garantindo que o nome usado no semáforo seja idêntico ao nome usado na escolha do encoder.
