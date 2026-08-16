# Sistema de Design do Painel de Controle — "Signal Path"

Este documento fixa a identidade visual aprovada para o painel de controle web do CodecAny (ver [`docs/propostas_painel_controle.md`](propostas_painel_controle.md) para arquitetura/API/fases). É a referência a seguir na implementação do frontend (`/web`) — qualquer componente novo deve derivar destes tokens em vez de introduzir cor/tipografia ad-hoc.

**Mockup de referência**: https://claude.ai/code/artifact/a5ee4548-557e-4c52-b7ed-e8f7be985a92 (paleta, tipografia, chips, medidores e a tela de Dashboard completa).

## Conceito

Um console técnico para operar o CodecAny — escuro, denso, feito para ficar aberto ao lado do servidor de mídia. Duas cores de acento com papéis **deliberadamente diferentes**, para não competirem entre si:

- **Roxo** — identidade e estado categórico: LED da marca, item de navegação ativo, chip "Convertendo" (pulsante), badges ativos. É a cor de "isto é o CodecAny" e "isto está em processamento agora".
- **Cyano** — leitura de sinal ao vivo, e só isso: preenchimento dos medidores de progresso, segmentos de utilização de hwaccel, o ponto do ticker de atividade, trechos de dados monoespaçados em destaque. É o traço de osciloscópio no escuro — nunca usado para navegação/identidade.

Dados técnicos (paths, codecs, tamanhos, timecodes) sempre em monoespaçada, como leitura de instrumento, com `font-variant-numeric: tabular-nums` em qualquer lugar onde números se alinham em coluna.

## Paleta (tokens)

| Token | Hex | Papel |
|---|---|---|
| `--ground` | `#0F0B17` | fundo da página |
| `--surface` | `#17111F` | painéis/cards |
| `--surface-2` | `#201929` | linhas de tabela, hover |
| `--surface-3` | `#2A2136` | elementos ainda mais elevados |
| `--border` | `#362A45` | divisores visíveis |
| `--border-soft` | `#251C32` | divisores sutis |
| `--text` | `#EEE8F7` | texto primário |
| `--text-muted` | `#A99CC4` | texto secundário |
| `--text-faint` | `#6D6184` | timestamps, legendas |
| `--accent` (roxo) | `#8C6FF2` | marca/identidade/estado ativo |
| `--accent-ink` | `#150A28` | texto sobre preenchimento roxo |
| `--accent-dim` / `--accent-fill` | `#4A3780` / `#271A44` | bordas/fundos de chip roxo |
| `--signal` (cyano) | `#3FD6E6` | leitura ao vivo (medidores, contadores) |
| `--signal-dim` / `--signal-fill` | `#1F5C66` / `#12303A` | variantes de fundo do cyano (uso raro) |
| `--success` | `#5AD394` | job concluído |
| `--danger` | `#F16B63` | job falhou |
| `--info` | `#8993D1` | na fila / aguardando (estado calmo, não é o cyano) |
| `--neutral` | `#8A7FA0` | revertido (rollback é resultado esperado, não erro) |

As semânticas (`success`/`danger`/`info`/`neutral`) são hues **separadas** dos dois acentos de marca — nunca reaproveitar roxo/cyano para comunicar status de job, para não confundir "isto é a marca" com "isto é o resultado deste job".

## Tipografia

Três papéis, duas famílias (mesma lógica de "família técnica coordenada" que projetos como IBM Plex usam):

1. **Display/labels** — Fira Sans Compressed (peso 800/600), maiúscula, tracking largo. Nav, headers de painel, labels de stat tile.
2. **Corpo/UI** — Fira Sans (400/500/600). Texto corrido, formulários, descrições.
3. **Dados/leitura** — Hack (400/700), monoespaçada. Paths, codecs, tamanhos, IDs de job, qualquer coluna numérica.

No mockup os três estão embutidos como WOFF2 (subset Latin + Latin Extended-A, ~35 KB cada) via `@font-face`/data URI — restrição específica do artifact (CSP bloqueia CDN de fonte). **Na implementação real do `/web`, servir os arquivos de fonte como estáticos normais** (Vite cuida do hashing/cache), sem necessidade de subsetting agressivo nem data URI — filenames de mídia real podem conter qualquer caractere Unicode, então usar a fonte completa (ou um subset amplo Latin+Latin Extended, não o subset mínimo do mockup).

## Componentes-chave

- **Chip de status**: pílula pequena, label em Display uppercase, ponto colorido à esquerda. Seis variantes (fila/progresso/aprovação/concluído/falhou/revertido) — `chip-progress` é a única com o ponto pulsante (`@keyframes pulse`, respeita `prefers-reduced-motion`).
- **Medidor de sinal**: barra segmentada (textura de "corte" via `repeating-linear-gradient` na cor do `--ground` sobre o preenchimento cyano) em vez de gradiente contínuo — reforça "sinal" em vez de "porcentagem genérica". Mesma lógica se aplica aos blocos discretos de utilização de hwaccel (N blocos = N slots).
- **Layout**: rail lateral fixo (nav com ícones lineares simples, sem emoji), barra superior com ticker de status ao vivo, conteúdo em grid denso (`gap`, nunca margin colapsável).

## Escopo deste documento

Cobre só os tokens e padrões validados no mockup de Dashboard. Telas com componentes novos (tabelas com filtro/paginação da Fila, formulários do builder de Regras, drag-reorder) precisam de um passo de design equivalente antes da implementação — reaproveitando sempre estes tokens, nunca introduzindo paleta/tipografia paralela.
