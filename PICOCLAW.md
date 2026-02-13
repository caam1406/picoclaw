# PicoClaw - Documentacao Completa do Projeto

> Documento de referencia para agentes AI e desenvolvedores.
> Contem toda a arquitetura, estruturas de dados, fluxos e APIs do projeto.
> **Versao**: 0.1.0 | **Licenca**: MIT | **Linguagem**: Go

---

## Indice

1. [Visao Geral](#1-visao-geral)
2. [Estrutura de Arquivos](#2-estrutura-de-arquivos)
3. [Arquitetura](#3-arquitetura)
4. [Ponto de Entrada (main.go)](#4-ponto-de-entrada)
5. [Message Bus (pkg/bus)](#5-message-bus)
6. [Agent Loop (pkg/agent)](#6-agent-loop)
7. [Channels (pkg/channels)](#7-channels)
8. [Tools (pkg/tools)](#8-tools)
9. [Skills (pkg/skills)](#9-skills)
10. [Contacts e Default Instructions (pkg/contacts)](#10-contacts)
11. [Session Manager (pkg/session)](#11-session-manager)
12. [Providers LLM (pkg/providers)](#12-providers)
13. [Dashboard Web (pkg/dashboard)](#13-dashboard)
14. [Storage Layer (pkg/storage)](#14-storage)
15. [Cron Service (pkg/cron)](#15-cron)
16. [Config (pkg/config)](#16-config)
17. [Servicos Auxiliares](#17-servicos-auxiliares)
18. [APIs REST do Dashboard](#18-apis-rest)
19. [Fluxo Completo de Mensagem](#19-fluxo-completo)

---

## 1. Visao Geral

PicoClaw e um agente AI ultra-leve escrito em Go. Conecta-se a multiplos canais de mensagem (WhatsApp, Telegram, Discord, etc.), processa mensagens com LLMs, executa tools e mantem contexto de conversacao.

**Repositorio**: `github.com/sipeed/picoclaw`
**Config**: `~/.picoclaw/config.json`
**Workspace**: `~/.picoclaw/workspace/`

### Componentes Principais

```
┌─────────────────────────────────────────────────────────────┐
│                        PicoClaw                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────┐   ┌───────────┐   ┌──────────────┐            │
│  │ Channels │──>│ MessageBus│──>│  Agent Loop   │            │
│  │ (7 tipos)│<──│ (pub/sub) │<──│ (LLM + Tools) │            │
│  └─────────┘   └───────────┘   └──────────────┘            │
│       │              │                │                      │
│       │              │         ┌──────┴──────┐              │
│       │              │         │ContextBuilder│              │
│       │              │         │ (prompt+skills│              │
│       │              │         │  +contacts)   │              │
│       │              │         └─────────────┘              │
│       │              │                                       │
│  ┌────┴────┐   ┌─────┴─────┐   ┌───────────┐              │
│  │Dashboard│   │  Session   │   │  Storage   │              │
│  │(Web+WS) │   │  Manager   │   │(File/PG)  │              │
│  └─────────┘   └───────────┘   └───────────┘              │
│                                                              │
│  ┌─────────┐   ┌───────────┐   ┌───────────┐              │
│  │  Cron   │   │ Heartbeat │   │   Voice    │              │
│  │ Service │   │  Service  │   │Transcriber │              │
│  └─────────┘   └───────────┘   └───────────┘              │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Estrutura de Arquivos

```
picoclaw-main/
├── cmd/picoclaw/
│   ├── main.go              # Entry point, CLI, gateway, onboard
│   └── migrate.go           # Migracao de dados entre backends
├── pkg/
│   ├── agent/
│   │   ├── loop.go          # Agent loop principal (processMessage, runLLMIteration)
│   │   ├── context.go       # ContextBuilder (system prompt, skills, contacts)
│   │   └── memory.go        # Memory store para workspace
│   ├── bus/
│   │   ├── bus.go           # MessageBus pub/sub com observers
│   │   └── types.go         # InboundMessage, OutboundMessage, BusEvent
│   ├── channels/
│   │   ├── base.go          # Channel interface + BaseChannel
│   │   ├── manager.go       # Manager (lifecycle, dispatch outbound)
│   │   ├── whatsapp.go      # WhatsApp via whatsmeow (nativo)
│   │   ├── telegram.go      # Telegram Bot API (long-polling)
│   │   ├── discord.go       # Discord via discordgo
│   │   ├── feishu.go        # Feishu/Lark via SDK WebSocket
│   │   ├── dingtalk.go      # DingTalk Stream Mode
│   │   ├── qq.go            # QQ via Tencent BotGo
│   │   └── maixcam.go       # MaixCam TCP server
│   ├── config/
│   │   └── config.go        # Config structs, LoadConfig, SaveConfig
│   ├── contacts/
│   │   └── store.go         # ContactInstruction + Default Instructions store
│   ├── cron/
│   │   └── service.go       # CronService, CronJob, scheduling
│   ├── dashboard/
│   │   ├── server.go        # HTTP server, auth, CORS, rotas
│   │   ├── handlers.go      # API handlers (contacts, defaults, storage, send)
│   │   ├── websocket.go     # WebSocket hub para live messages
│   │   ├── static.go        # go:embed frontend/*
│   │   └── frontend/
│   │       ├── index.html   # Dashboard HTML
│   │       ├── app.js       # Dashboard JS (API calls, CRUD, WebSocket)
│   │       └── style.css    # Dark theme CSS
│   ├── heartbeat/
│   │   └── service.go       # Heartbeat periodico (proactive tasks)
│   ├── logger/
│   │   ├── logger.go        # Zerolog structured logging
│   │   └── logger_test.go   # Testes
│   ├── providers/
│   │   ├── types.go         # LLMProvider interface, Message, ToolCall
│   │   └── http_provider.go # HTTPProvider (OpenAI-compatible API)
│   ├── session/
│   │   └── manager.go       # SessionManager (history, summary, persist)
│   ├── skills/
│   │   ├── loader.go        # SkillsLoader (workspace > global > builtin)
│   │   └── installer.go     # Install/uninstall skills
│   ├── storage/
│   │   ├── interface.go     # Storage interface + Config
│   │   ├── factory.go       # NewStorage() factory
│   │   ├── file/            # File-based JSON storage
│   │   ├── postgres/        # PostgreSQL storage + migrations
│   │   └── repository/      # Repository interfaces (Session, Contacts, Cron)
│   ├── tools/
│   │   ├── base.go          # Tool interface
│   │   ├── registry.go      # ToolRegistry (register, execute, definitions)
│   │   ├── filesystem.go    # read_file, write_file, list_dir
│   │   ├── shell.go         # exec (com safety guards)
│   │   ├── edit.go          # edit_file, append_file
│   │   ├── web.go           # web_search (Brave API), web_fetch
│   │   ├── message.go       # message (send to channel)
│   │   ├── spawn.go         # spawn (subagent)
│   │   ├── subagent.go      # SubagentManager
│   │   ├── cron.go          # cron tool (schedule tasks)
│   │   └── types.go         # ToolCall, ToolDefinition types
│   ├── utils/
│   │   └── string.go        # Truncate()
│   └── voice/
│       └── transcriber.go   # GroqTranscriber (Whisper)
├── skills/                   # Built-in skills
│   ├── github/SKILL.md
│   ├── weather/SKILL.md
│   ├── summarize/SKILL.md
│   ├── tmux/SKILL.md + scripts/
│   └── skill-creator/SKILL.md
├── config.example.json
├── Makefile
├── go.mod / go.sum
└── README.md
```

---

## 3. Arquitetura

### Fluxo de Dados Principal

```
Inbound:  Channel -> MessageBus.PublishInbound() -> AgentLoop.processMessage()
                                                         |
                                                    ContactGate (contacts_only?)
                                                         |
                                                    runAgentLoop()
                                                         |
                                                    ContextBuilder.BuildMessages()
                                                    (system prompt + contact/default instructions + history + skills)
                                                         |
                                                    LLM Provider.Chat()
                                                         |
                                                    Tool Execution (se necessario)
                                                         |
Outbound: AgentLoop -> MessageBus.PublishOutbound() -> Manager.dispatchOutbound() -> Channel.Send()
```

### Hierarquia de Instrucoes

Quando uma mensagem chega, o sistema injeta instrucoes no system prompt nesta ordem de prioridade:

1. **Contato registrado**: Se o sender tem uma `ContactInstruction`, usa ela
2. **Default por canal**: Se nao e contato, tenta `defaults[channel]` (ex: "whatsapp")
3. **Default global**: Se nao tem por canal, usa `defaults["*"]`
4. **Nenhuma**: Prompt padrao sem instrucoes extras

---

## 4. Ponto de Entrada

**Arquivo**: `cmd/picoclaw/main.go`

### Comandos CLI

| Comando | Descricao |
|---------|-----------|
| `onboard` | Setup inicial (cria config, workspace, templates) |
| `agent` | Modo CLI interativo (conversa direta) |
| `gateway` | Daemon persistente (multi-channel + dashboard) |
| `status` | Mostra estado do sistema |
| `cron list/add/remove/enable/disable` | Gerencia tarefas agendadas |
| `skills list/install/remove/show` | Gerencia skills |
| `version` | Mostra versao |

### Sequencia de Inicializacao (gateway)

1. Carrega config (`~/.picoclaw/config.json`)
2. Cria LLM Provider (HTTPProvider)
3. Cria MessageBus (buffers: 100 inbound, 100 outbound)
4. Cria AgentLoop (com ToolRegistry: 10+ tools)
5. Setup CronService + CronTool
6. Setup HeartbeatService (30min interval)
7. Cria ChannelManager + inicializa canais habilitados
8. Opcional: configura voice transcription (Groq Whisper)
9. Inicia todos os servicos (cron, heartbeat, channels, agent loop)
10. Se dashboard enabled: cria ContactsStore, Dashboard Server
11. Aguarda signal (Ctrl+C) para graceful shutdown

### Workspace Templates Criados no Onboard

| Arquivo | Descricao |
|---------|-----------|
| `AGENTS.md` | Instrucoes de comportamento do agente |
| `SOUL.md` | Personalidade e valores |
| `USER.md` | Informacoes do usuario |
| `IDENTITY.md` | Identidade completa do sistema |
| `memory/MEMORY.md` | Memoria de longo prazo |

---

## 5. Message Bus

**Arquivo**: `pkg/bus/bus.go`, `pkg/bus/types.go`

### Structs

```go
type InboundMessage struct {
    Channel    string            // "telegram", "whatsapp", etc.
    SenderID   string            // ID do sender
    ChatID     string            // ID do chat/grupo
    Content    string            // Texto da mensagem
    Media      []string          // Paths de midia
    SessionKey string            // "channel:chatID"
    Metadata   map[string]string // Dados extras do canal
}

type OutboundMessage struct {
    Channel string // Canal destino
    ChatID  string // Chat destino
    Content string // Texto da resposta
}

type BusEvent struct {
    Type     string            // "inbound" ou "outbound"
    Inbound  *InboundMessage
    Outbound *OutboundMessage
    Time     time.Time
}

type MessageBus struct {
    inbound   chan InboundMessage  // buffer: 100
    outbound  chan OutboundMessage // buffer: 100
    handlers  map[string]MessageHandler
    observers []chan BusEvent      // buffer: 50 cada
}
```

### Metodos

| Metodo | Descricao |
|--------|-----------|
| `PublishInbound(msg)` | Canal publica mensagem recebida |
| `ConsumeInbound(ctx)` | Agent consome mensagem (blocking) |
| `PublishOutbound(msg)` | Agent publica resposta |
| `SubscribeOutbound(ctx)` | Canal consome resposta (blocking) |
| `Subscribe()` | Dashboard subscribe para live events |
| `Unsubscribe(ch)` | Dashboard unsubscribe |
| `RegisterHandler(channel, handler)` | Registra handler de canal |

---

## 6. Agent Loop

**Arquivos**: `pkg/agent/loop.go`, `pkg/agent/context.go`, `pkg/agent/memory.go`

### AgentLoop Struct

```go
type AgentLoop struct {
    bus            *bus.MessageBus
    provider       providers.LLMProvider
    workspace      string
    model          string           // ex: "glm-4.7"
    contextWindow  int              // max tokens
    maxIterations  int              // max tool call loops
    sessions       *session.SessionManager
    contextBuilder *ContextBuilder
    tools          *tools.ToolRegistry
    contactsStore  *contacts.Store
    contactsOnly   bool             // gate: so contatos registrados
}
```

### processMessage() - Fluxo Principal

```
1. Log da mensagem recebida
2. Se channel == "system" -> processSystemMessage()
3. Contact gate: se contactsOnly=true e sender nao registrado -> ignora silenciosamente
   (bypass para channels "cli" e "cron")
4. runAgentLoop(processOptions)
```

### runAgentLoop() - Core do Processamento

```
1. updateToolContexts(channel, chatID) - injeta contexto em message/spawn tools
2. history = sessions.GetHistory(sessionKey)
3. summary = sessions.GetSummary(sessionKey)
4. messages = contextBuilder.BuildMessages(history, summary, userMessage, media, channel, chatID)
5. sessions.AddMessage(sessionKey, "user", userMessage)
6. runLLMIteration(ctx, messages, opts) - loop LLM + tools
7. Se resposta vazia -> usar defaultResponse
8. sessions.AddMessage(sessionKey, "assistant", finalContent)
9. maybeSummarize(sessionKey) - se history > 20 msgs ou > 75% context window
10. Se sendResponse=true -> publishOutbound
```

### runLLMIteration() - Loop LLM + Tools

```
for iteration < maxIterations:
    1. Build tool definitions from registry
    2. provider.Chat(ctx, messages, toolDefs, model, {max_tokens:8192, temp:0.7})
    3. Se sem tool calls -> finalContent = response.Content, break
    4. Se com tool calls:
       a. Append assistant message com tool calls
       b. Para cada tool call:
          - tools.ExecuteWithContext(ctx, name, args, channel, chatID)
          - Append tool result message
       c. Continue loop (LLM vera os resultados)
```

### ContextBuilder - Construcao do System Prompt

```go
type ContextBuilder struct {
    workspace     string
    skillsLoader  *skills.SkillsLoader
    memory        *MemoryStore
    tools         *tools.ToolRegistry
    contactsStore *contacts.Store
}
```

**BuildSystemPrompt()** monta o prompt com:
1. **Identity**: nome, hora, runtime, workspace, regras
2. **Bootstrap Files**: AGENTS.md, SOUL.md, USER.md, IDENTITY.md
3. **Skills Summary**: XML com skills disponiveis
4. **Memory Context**: conteudo de memory/MEMORY.md

**BuildMessages()** adiciona ao prompt:
1. System prompt (acima)
2. Current Session info (channel, chatID)
3. **Contact/Default Instructions** (hierarquia de prioridade)
4. Summary of Previous Conversation (se existir)
5. History messages
6. User message atual

### Injecao de Instrucoes (context.go, linhas 176-183)

```go
if cb.contactsStore != nil {
    sessionKey := fmt.Sprintf("%s:%s", channel, chatID)
    if instruction := cb.contactsStore.GetForSession(sessionKey); instruction != "" {
        // Contato registrado: usa instrucao do contato
        systemPrompt += "\n\n## Contact-Specific Instructions\n\n" + instruction
    } else if defaultInst := cb.contactsStore.GetDefault(channel); defaultInst != "" {
        // Nao-contato: usa default (canal especifico ou global "*")
        systemPrompt += "\n\n## Default Instructions\n\n" + defaultInst
    }
}
```

### Sumarizacao

Ativada quando `history > 20 messages` ou `tokens > 75% contextWindow`.
- Divide em 2 partes se > 10 messages
- Sumariza cada parte separadamente
- Merge summaries
- Trunca history mantendo ultimas 4 mensagens

---

## 7. Channels

**Arquivos**: `pkg/channels/*.go`

### Channel Interface

```go
type Channel interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Send(ctx context.Context, msg bus.OutboundMessage) error
    IsRunning() bool
    IsAllowed(senderID string) bool
}
```

### BaseChannel

Implementacao base compartilhada por todos os canais:
- `HandleMessage(senderID, chatID, content, media, metadata)` -> publica `InboundMessage` no bus
- `IsAllowed(senderID)` -> checa allowList (vazio = permite todos)
- SessionKey gerado como `"channel:chatID"`

### Manager

```go
type Manager struct {
    channels map[string]Channel
    bus      *bus.MessageBus
}
```

- `StartAll(ctx)` -> inicia todos os canais + goroutine `dispatchOutbound()`
- `dispatchOutbound()` -> consome `bus.SubscribeOutbound()` e roteia para `channel.Send()`
- `GetStatus()` -> retorna mapa de status por canal
- `SendToChannel(ctx, channelName, chatID, content)` -> envia para canal especifico

### Canais Implementados

| Canal | Biblioteca | Protocolo | Especificidades |
|-------|-----------|-----------|-----------------|
| **WhatsApp** | whatsmeow | WebSocket nativo | QR login, SQLite session, reconnect loop, voice PTT, media download |
| **Telegram** | telegram-bot-api | Long-polling | Thinking animation, markdown->HTML, photo/voice/doc, placeholder editing |
| **Discord** | discordgo | WebSocket | Audio detection, attachment handling |
| **Feishu** | lark SDK | WebSocket | EventDispatcher, encrypt key |
| **DingTalk** | dingtalk SDK | Stream Mode | SessionWebhook, Markdown reply |
| **QQ** | Tencent BotGo | WebSocket | Token refresh, C2C + Group AT, deduplication |
| **MaixCam** | TCP custom | TCP JSON | Person detection, hardware commands |

### WhatsApp - Detalhes

- **Store**: SQLite em `~/.picoclaw/whatsapp.db` (WAL mode, single conn)
- **Login**: QR code no terminal (primeira vez)
- **Reconnect**: Loop a cada 10s, backoff exponencial 5s -> 5min
- **Media**: Download de image/audio/video/document/sticker para temp
- **Voice**: PTT transcription via GroqTranscriber (se disponivel)
- **Send**: Typing indicator antes, envia texto

### Telegram - Detalhes

- **Updates**: Long-polling com timeout 30s
- **Thinking**: Placeholder "Thinking... :thought_balloon:" com animacao de dots a cada 2s
- **Send**: Edita placeholder se existir, senao envia nova mensagem
- **Markdown**: Converte para Telegram HTML (bold, italic, code blocks, links)
- **Media**: Download de Photo, Voice, Audio, Document

---

## 8. Tools

**Arquivos**: `pkg/tools/*.go`

### Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{} // JSON Schema
    Execute(ctx context.Context, args map[string]interface{}) (string, error)
}
```

### ToolRegistry

```go
type ToolRegistry struct {
    tools map[string]Tool
    mu    sync.RWMutex
}
```

- `Register(tool)` / `Get(name)` / `List()` / `GetDefinitions()`
- `Execute(ctx, name, args)` -> executa tool com logging
- `ExecuteWithContext(ctx, name, args, channel, chatID)` -> injeta contexto de sessao
- `GetSummaries()` -> lista "name - description" para system prompt

### Tools Disponiveis

| Tool | Arquivo | Parametros | Descricao |
|------|---------|-----------|-----------|
| `read_file` | filesystem.go | `path` (string) | Le conteudo de arquivo |
| `write_file` | filesystem.go | `path`, `content` (string) | Escreve arquivo (cria dirs) |
| `list_dir` | filesystem.go | `path` (string, opt) | Lista diretorio com DIR:/FILE: prefix |
| `exec` | shell.go | `command`, `working_dir` (opt) | Executa comando shell (60s timeout, safety guards) |
| `edit_file` | edit.go | `path`, `old_text`, `new_text` | Substitui texto exato (match unico) |
| `append_file` | edit.go | `path`, `content` | Adiciona conteudo ao final |
| `web_search` | web.go | `query`, `count` (1-10) | Busca web via Brave API |
| `web_fetch` | web.go | `url`, `maxChars` (opt, 50k) | Fetch + extract HTML to text |
| `message` | message.go | `content`, `channel` (opt), `chat_id` (opt) | Envia mensagem para canal |
| `spawn` | spawn.go | `task`, `label` (opt) | Cria subagent em goroutine |
| `cron` | cron.go | `action`, `message`, scheduling params | Agenda tarefas |

### exec - Safety Guards

**Bloqueados** (regex deny patterns):
- `rm -rf /`, `rm -rf ~`, `rm -rf .`
- `dd if=`, `mkfs`, `shutdown`, `reboot`
- Fork bombs, `chmod -R 777 /`, `chown -R`
- `curl|sh`, `wget|sh` (pipe to shell)

**Limites**:
- Timeout: 60 segundos
- Output max: 10.000 caracteres
- Opcional: restrict to workspace directory

---

## 9. Skills

**Arquivos**: `pkg/skills/loader.go`, `pkg/skills/installer.go`

### Hierarquia de Busca (prioridade decrescente)

1. **Workspace**: `~/.picoclaw/workspace/skills/`
2. **Global**: `~/.picoclaw/skills/`
3. **Builtin**: `./skills/` (no diretorio do binario)

### Formato SKILL.md

```yaml
---
name: skill_name
description: "Descricao breve"
metadata: {"nanobot":{"emoji":"icon","requires":{"bins":["tool"]}}}
---

# Conteudo Markdown

Instrucoes, exemplos, comandos...
```

### SkillsLoader

```go
type SkillsLoader struct {
    workspace     string // workspace/skills
    globalSkills  string // ~/.picoclaw/skills
    builtinSkills string // ./skills
}
```

- `ListSkills()` -> lista todas com source (workspace/global/builtin)
- `LoadSkill(name)` -> carrega conteudo markdown (strip frontmatter)
- `BuildSkillsSummary()` -> XML summary para system prompt

### Skills Builtin

| Skill | Descricao | Requer |
|-------|-----------|--------|
| `github` | GitHub CLI (PRs, issues, workflows) | `gh` |
| `weather` | Clima via wttr.in e Open-Meteo | `curl` |
| `summarize` | Sumarizacao de URLs/PDFs/videos | `summarize` CLI |
| `tmux` | Controle de sessoes tmux | `tmux` |
| `skill-creator` | Meta-skill para criar novos skills | nenhum |

---

## 10. Contacts e Default Instructions

**Arquivo**: `pkg/contacts/store.go`

### ContactInstruction (instrucoes por contato)

```go
type ContactInstruction struct {
    ContactID    string    `json:"contact_id"`
    Channel      string    `json:"channel"`
    DisplayName  string    `json:"display_name"`
    Instructions string    `json:"instructions"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

**Armazenamento**: `~/.picoclaw/workspace/contacts/instructions.json`
**Chave**: `"channel:contactID"` (ex: `"whatsapp:5511982650676"`)

### Default Instructions (instrucoes para nao-contatos)

```go
defaults map[string]string // key: canal ou "*" para global
```

**Armazenamento**: `~/.picoclaw/workspace/contacts/defaults.json`

**Formato**:
```json
{
  "*": "Instrucao global para todos os nao-contatos",
  "whatsapp": "Instrucao especifica para WhatsApp",
  "telegram": "Instrucao especifica para Telegram"
}
```

### Store

```go
type Store struct {
    mu               sync.RWMutex
    instructions     map[string]*ContactInstruction
    filePath         string
    defaults         map[string]string
    defaultsFilePath string
}
```

### Metodos - Contacts

| Metodo | Descricao |
|--------|-----------|
| `Get(channel, contactID)` | Busca contato |
| `Set(ci)` | Cria/atualiza contato |
| `Delete(channel, contactID)` | Remove contato |
| `List()` | Lista todos os contatos |
| `GetForSession(sessionKey)` | Busca instrucao por session key (com JID strip para WhatsApp) |
| `IsRegistered(sessionKey)` | Verifica se contato existe |
| `Count()` | Total de contatos |

### Metodos - Defaults

| Metodo | Descricao |
|--------|-----------|
| `GetDefault(channel)` | Busca: canal especifico -> fallback global "*" |
| `SetDefault(channel, instructions)` | Define instrucao (use "*" para global) |
| `DeleteDefault(channel)` | Remove instrucao |
| `ListDefaults()` | Retorna mapa completo |

### GetForSession - Logica de Lookup

```
1. Tenta match exato: instructions["whatsapp:5511982650676@s.whatsapp.net"]
2. Se WhatsApp: strip JID suffix, tenta instructions["whatsapp:5511982650676"]
3. Se nao encontrou: retorna ""
```

---

## 11. Session Manager

**Arquivo**: `pkg/session/manager.go`

```go
type Session struct {
    Key      string
    Messages []providers.Message
    Summary  string
    Created  time.Time
    Updated  time.Time
}

type SessionManager struct {
    sessions map[string]*Session
    mu       sync.RWMutex
    storage  string // diretorio para persistencia JSON
}
```

### Metodos

| Metodo | Descricao |
|--------|-----------|
| `GetOrCreate(key)` | Cria sessao se nao existir |
| `AddMessage(key, role, content)` | Adiciona mensagem simples |
| `AddFullMessage(key, msg)` | Adiciona mensagem completa (com tool calls) |
| `GetHistory(key)` | Retorna copia do historico |
| `GetSummary(key)` | Retorna summary da sessao |
| `SetSummary(key, summary)` | Define summary |
| `TruncateHistory(key, keepLast)` | Mantem ultimas N mensagens |
| `Save(session)` | Persiste em JSON |
| `ListSessions()` | Lista metadados de todas as sessoes |

---

## 12. Providers LLM

**Arquivos**: `pkg/providers/types.go`, `pkg/providers/http_provider.go`

### Interface

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []ToolDefinition,
         model string, options map[string]interface{}) (*LLMResponse, error)
    GetDefaultModel() string
}
```

### Message Types

```go
type Message struct {
    Role       string     // "user", "assistant", "system", "tool"
    Content    string
    ToolCalls  []ToolCall // Para assistant messages
    ToolCallID string     // Para tool response messages
}

type ToolCall struct {
    ID       string
    Type     string // "function"
    Function *FunctionCall
    Name     string
    Arguments map[string]interface{}
}

type LLMResponse struct {
    Content      string
    ToolCalls    []ToolCall
    FinishReason string
    Usage        *UsageInfo
}
```

### HTTPProvider

Implementacao generica compativel com API OpenAI. Suporta:

| Provider | Config Key | API Base |
|----------|-----------|----------|
| OpenRouter | `providers.openrouter.api_key` | `https://openrouter.ai/api/v1` |
| Anthropic | `providers.anthropic.api_key` | padrao |
| OpenAI | `providers.openai.api_key` | padrao |
| Gemini | `providers.gemini.api_key` | padrao |
| Zhipu | `providers.zhipu.api_key` | custom |
| Z.AI | `providers.zai.api_key` | `https://api.z.ai/api/paas/v4` |
| Groq | `providers.groq.api_key` | padrao |
| vLLM | `providers.vllm.api_key` | custom |

**Prioridade de selecao**: OpenRouter > Anthropic > OpenAI > Gemini > Zhipu > ZAI > Groq > VLLM

---

## 13. Dashboard Web

**Arquivos**: `pkg/dashboard/server.go`, `pkg/dashboard/handlers.go`, `pkg/dashboard/websocket.go`

### Server

```go
type Server struct {
    config         config.DashboardConfig
    cfg            *config.Config
    channelManager *channels.Manager
    sessions       *session.SessionManager
    contactsStore  *contacts.Store
    msgBus         *bus.MessageBus
    hub            *Hub            // WebSocket hub
    httpServer     *http.Server
}
```

**Autenticacao**: Bearer token no header `Authorization` (ou query param `?token=` para WebSocket)

### Frontend

SPA vanilla JS com dark theme. Embeddado via `go:embed frontend/*`.

**Views**:
- `view-overview`: Stats (canais, sessoes, contatos, uptime) + sessoes recentes
- `view-contact`: CRUD de contato com instrucoes personalizadas
- `view-default`: CRUD de instrucoes padrao para nao-contatos
- `view-live`: Mensagens em tempo real via WebSocket
- `view-settings`: Configuracao de storage (file/postgres)

**Modais**:
- `modal-add-contact`: Novo contato (canal, ID, nome)
- `modal-add-default`: Nova instrucao padrao (canal ou global)

---

## 14. Storage Layer

**Arquivos**: `pkg/storage/`

### Interface Principal

```go
type Storage interface {
    Sessions() repository.SessionRepository
    Contacts() repository.ContactsRepository
    Cron()     repository.CronRepository
    Connect(ctx context.Context) error
    Close() error
    Ping(ctx context.Context) error
}
```

### Backends

| Backend | Tipo | Persistencia |
|---------|------|-------------|
| **File** | `"file"` | JSON files em workspace |
| **PostgreSQL** | `"postgres"` | Banco relacional com JSONB |

### Schema PostgreSQL

**sessions**:
```sql
key VARCHAR(255) PRIMARY KEY
messages JSONB NOT NULL DEFAULT '[]'
summary TEXT
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```

**contact_instructions**:
```sql
channel VARCHAR(50) NOT NULL
contact_id VARCHAR(255) NOT NULL
display_name VARCHAR(255)
instructions TEXT NOT NULL
created_at, updated_at TIMESTAMP WITH TIME ZONE
PRIMARY KEY (channel, contact_id)
```

**cron_jobs**:
```sql
id VARCHAR(64) PRIMARY KEY
name VARCHAR(255) NOT NULL
enabled BOOLEAN DEFAULT true
schedule JSONB NOT NULL
payload JSONB NOT NULL
state JSONB DEFAULT '{}'
created_at_ms, updated_at_ms BIGINT
delete_after_run BOOLEAN DEFAULT false
```

### Migracao de Dados

Comando `picoclaw migrate` converte entre file e postgres:
- `migrateSessions()`, `migrateContacts()`, `migrateCronJobs()`
- `picoclaw migrate export <dir>` exporta para JSON

---

## 15. Cron Service

**Arquivo**: `pkg/cron/service.go`

### CronJob

```go
type CronJob struct {
    ID             string
    Name           string
    Enabled        bool
    Schedule       CronSchedule  // Kind: "at"|"every"|"cron"
    Payload        CronPayload   // Kind: "agent_turn", Message, Deliver, Channel, To
    State          CronJobState  // NextRunAtMS, LastRunAtMS, LastStatus
    CreatedAtMS    int64
    DeleteAfterRun bool
}
```

### CronService

- Loop a cada 1 segundo
- Verifica jobs com `nextRunAtMS <= now`
- Executa via callback `onJob(job)`
- Jobs "at" auto-deletam apos execucao
- Jobs "every" recalculam proximo run
- Jobs "cron" usam gronx parser para expressoes cron
- Persistencia em `workspace/cron/jobs.json`

---

## 16. Config

**Arquivo**: `pkg/config/config.go`

```go
type Config struct {
    Agents    AgentsConfig
    Channels  ChannelsConfig
    Providers ProvidersConfig
    Gateway   GatewayConfig
    Tools     ToolsConfig
    Dashboard DashboardConfig
    Storage   StorageConfig
}
```

### AgentDefaults

| Campo | Tipo | Default | Descricao |
|-------|------|---------|-----------|
| `workspace` | string | `~/.picoclaw/workspace` | Diretorio de trabalho |
| `model` | string | `glm-4.7` | Modelo LLM padrao |
| `max_tokens` | int | `8192` | Context window |
| `temperature` | float64 | `0.7` | Temperatura do LLM |
| `max_tool_iterations` | int | `20` | Max loops de tool calls |

### DashboardConfig

| Campo | Tipo | Default | Descricao |
|-------|------|---------|-----------|
| `enabled` | bool | `false` | Habilita dashboard web |
| `host` | string | `127.0.0.1` | Host do servidor |
| `port` | int | `18791` | Porta do servidor |
| `token` | string | `""` | Token de autenticacao |
| `contacts_only` | bool | `false` | So responde contatos registrados |

### StorageConfig

| Campo | Tipo | Default | Descricao |
|-------|------|---------|-----------|
| `type` | string | `"file"` | Backend: "file" ou "postgres" |
| `database_url` | string | `""` | URL do PostgreSQL |
| `file_path` | string | `~/.picoclaw/workspace/sessions` | Path para file storage |
| `ssl_enabled` | bool | `false` | SSL para PostgreSQL |

### Canais - Campos Comuns

Todos os canais tem: `enabled` (bool), `allow_from` ([]string)

| Canal | Campos Especificos |
|-------|--------------------|
| WhatsApp | `store_path` (SQLite) |
| Telegram | `token` |
| Discord | `token` |
| Feishu | `app_id`, `app_secret`, `encrypt_key`, `verification_token` |
| QQ | `app_id`, `app_secret` |
| DingTalk | `client_id`, `client_secret` |
| MaixCam | `host`, `port` |

---

## 17. Servicos Auxiliares

### Heartbeat (`pkg/heartbeat/service.go`)

- Executa a cada N segundos (default: 1800 = 30min)
- Le `workspace/memory/HEARTBEAT.md`
- Gera prompt com hora atual + notas
- Chama callback (processamento do agente)
- Log em `workspace/memory/heartbeat.log`

### Voice Transcriber (`pkg/voice/transcriber.go`)

- **API**: Groq Whisper (whisper-large-v3)
- **Input**: Arquivo de audio (qualquer formato)
- **Output**: `TranscriptionResponse{Text, Language, Duration}`
- Usado por WhatsApp (PTT), Telegram (Voice), Discord (Audio)
- Injetado via `channel.SetTranscriber(transcriber)`

### Logger (`pkg/logger/logger.go`)

- Dual output: console + optional JSON file
- Levels: DEBUG, INFO, WARN, ERROR, FATAL
- Structured: `InfoCF("component", "message", map[string]interface{}{})`
- Thread-safe com RWMutex
- Auto-extracts caller via runtime

---

## 18. APIs REST do Dashboard

**Base**: `http://localhost:18791`
**Auth**: `Authorization: Bearer <token>`

### Status e Canais

| Metodo | Path | Descricao |
|--------|------|-----------|
| GET | `/api/v1/status` | Versao, uptime, status dos canais |
| GET | `/api/v1/channels` | Status de cada canal (running/parado) |

### Sessoes

| Metodo | Path | Descricao |
|--------|------|-----------|
| GET | `/api/v1/sessions` | Lista todas as sessoes |
| GET | `/api/v1/sessions/{key}` | Detalhe de uma sessao |

### Contatos

| Metodo | Path | Descricao |
|--------|------|-----------|
| GET | `/api/v1/contacts` | Lista todos os contatos |
| GET | `/api/v1/contacts/{channel}/{id}` | Detalhe de um contato |
| PUT | `/api/v1/contacts/{channel}/{id}` | Cria/atualiza contato |
| DELETE | `/api/v1/contacts/{channel}/{id}` | Remove contato |

**Body PUT**: `{"display_name": "Nome", "instructions": "Instrucoes..."}`

### Default Instructions

| Metodo | Path | Descricao |
|--------|------|-----------|
| GET | `/api/v1/defaults` | Lista todas as instrucoes padrao |
| GET | `/api/v1/defaults/{channel}` | Instrucao de um canal (use `*` para global) |
| PUT | `/api/v1/defaults/{channel}` | Cria/atualiza instrucao padrao |
| DELETE | `/api/v1/defaults/{channel}` | Remove instrucao padrao |

**Body PUT**: `{"instructions": "Instrucao..."}`

### Envio de Mensagens

| Metodo | Path | Descricao |
|--------|------|-----------|
| POST | `/api/v1/send` | Envia mensagem para um canal |

**Body**: `{"channel": "whatsapp", "chat_id": "5511...", "content": "Ola!"}`

### Storage Config

| Metodo | Path | Descricao |
|--------|------|-----------|
| GET | `/api/v1/config/storage` | Config atual (password mascarado) |
| PUT | `/api/v1/config/storage/update` | Atualiza config (requer restart) |
| POST | `/api/v1/config/storage/test` | Testa conexao com database |

### WebSocket

| Path | Descricao |
|------|-----------|
| `/ws?token=<token>` | Stream de eventos em tempo real (inbound/outbound) |

**Formato do evento**:
```json
{
  "type": "inbound",
  "inbound": {"channel": "whatsapp", "sender_id": "...", "content": "..."},
  "time": "2026-02-13T..."
}
```

---

## 19. Fluxo Completo de Mensagem

### Exemplo: Mensagem WhatsApp de nao-contato

```
1. Usuario envia "Ola" via WhatsApp
2. whatsmeow recebe evento Message
3. WhatsAppChannel.handleIncomingMessage():
   - Extrai texto, senderID, chatID
   - BaseChannel.HandleMessage() cria InboundMessage:
     {Channel:"whatsapp", SenderID:"5511...", ChatID:"5511...@s.whatsapp.net",
      Content:"Ola", SessionKey:"whatsapp:5511...@s.whatsapp.net"}
4. MessageBus.PublishInbound(msg) -> notifica observers (dashboard)

5. AgentLoop.Run() -> ConsumeInbound() recebe msg
6. processMessage():
   - Nao e "system" -> continua
   - contactsOnly=false -> nao bloqueia (ou contactsOnly=true e IsRegistered)
7. runAgentLoop():
   a. updateToolContexts("whatsapp", "5511...@s.whatsapp.net")
   b. GetHistory("whatsapp:5511...@s.whatsapp.net") -> []
   c. GetSummary("whatsapp:5511...@s.whatsapp.net") -> ""
   d. BuildMessages():
      - BuildSystemPrompt() -> identity + bootstrap files + skills + memory
      - "## Current Session\nChannel: whatsapp\nChat ID: 5511..."
      - GetForSession("whatsapp:5511...@s.whatsapp.net") -> "" (nao e contato)
      - GetDefault("whatsapp") -> "Responda de forma breve" (ou fallback "*")
      - Injeta: "## Default Instructions\n\nResponda de forma breve"
   e. AddMessage("user", "Ola")
   f. runLLMIteration():
      - provider.Chat(messages, tools, "glm-4.7", {max_tokens:8192, temp:0.7})
      - LLM retorna: "Ola! Como posso ajudar?"
   g. AddMessage("assistant", "Ola! Como posso ajudar?")
   h. maybeSummarize() -> skip (poucos msgs)

8. AgentLoop.Run() -> PublishOutbound({Channel:"whatsapp", ChatID:"5511...", Content:"Ola!..."})
9. Manager.dispatchOutbound() -> WhatsAppChannel.Send():
   - Envia typing indicator
   - Envia mensagem via whatsmeow
   - Para typing
10. Usuario recebe "Ola! Como posso ajudar?" no WhatsApp
```
