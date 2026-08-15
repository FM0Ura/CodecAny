# Lista de Tarefas: CodecAny v1.1

## 🏁 Fase 1: Aceleração de Hardware e Relatório de Economia

- [x] **Tarefa 1**: Modelagem e Parsing das Regras de HW-Accel
  - **Arquivos**: `pkg/core/types.go`, `pkg/core/rules.go`
- [x] **Tarefa 2**: Mapeamento de Argumentos de HW-Accel no Adaptador FFmpeg
  - **Arquivos**: `pkg/adapters/ffmpeg/transcode.go`, `pkg/adapters/ffmpeg/transcode_test.go`
- [x] **Tarefa 3**: Lógica de Banco de Dados de Economia Acumulada
  - **Arquivos**: `pkg/core/store.go`
- [x] **Tarefa 4**: Exibição do Relatório de Economia no CLI
  - **Arquivos**: `cmd/cli/main.go`

### 🚩 Checkpoint: Hardware & Economia
- [x] Compilação do CLI e Core passam sem erros.
- [x] Testes unitários de hardware e store passam.
- [x] CLI imprime sumário ao terminar execução.

---

## 🏁 Fase 2: Legendas Externas e Webhooks

- [x] **Tarefa 5**: Descoberta de Legendas Irmãs no Diretório (Probe)
  - **Arquivos**: `pkg/core/types.go`, `pkg/adapters/ffmpeg/probe.go`
- [x] **Tarefa 6**: Remuxing de Legendas no Adaptador FFmpeg
  - **Arquivos**: `pkg/adapters/ffmpeg/transcode.go`, `pkg/adapters/ffmpeg/transcode_test.go`
- [x] **Tarefa 7**: Cliente HTTP de Webhook e Configuração Declarativa
  - **Arquivos**: `pkg/core/rules.go`, `pkg/core/events.go`, `pkg/core/webhook.go`
- [x] **Tarefa 8**: Despacho Assíncrono de Eventos de Webhook no Engine
  - **Arquivos**: `pkg/core/engine.go`

### 🚩 Checkpoint: Legendas & Webhooks
- [x] Todos os testes unitários e de integração do sistema passam.
- [x] Envio assíncrono de webhook verificado.
- [x] Legendas mapeadas e embutidas corretamente no container.
