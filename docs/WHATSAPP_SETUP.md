# WhatsApp Setup - PicoClaw

Guia completo para configurar e testar o canal WhatsApp nativo do PicoClaw.

## Pre-requisitos

- Go 1.24+ instalado
- Uma conta WhatsApp ativa no celular
- Pelo menos um provider de LLM configurado (OpenRouter, Anthropic, OpenAI, etc.)
- (Opcional) API key do Groq para transcricao de audio

## Passo 1: Compilar o projeto

```bash
cd C:\Users\Administrator\Documents\Projetos\picoclaw-main\picoclaw-main
go build -o picoclaw.exe ./cmd/picoclaw/
```

## Passo 2: Criar a configuracao inicial

```bash
.\picoclaw.exe onboard
```

Isso cria o arquivo `C:\Users\Administrator\.picoclaw\config.json` com valores padrao.

## Passo 3: Editar o config.json

Abra `C:\Users\Administrator\.picoclaw\config.json` e habilite o WhatsApp:

```json
{
  "channels": {
    "whatsapp": {
      "enabled": true,
      "store_path": "~/.picoclaw/whatsapp.db",
      "allow_from": ["5511999999999"]
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "sk-or-v1-SUA_CHAVE_AQUI"
    }
  }
}
```

### Campos do WhatsApp

| Campo | Descricao | Exemplo |
|-------|-----------|---------|
| `enabled` | Ativa o canal WhatsApp | `true` |
| `store_path` | Caminho do banco SQLite para sessao | `~/.picoclaw/whatsapp.db` |
| `allow_from` | Lista de numeros autorizados (sem +) | `["5511982650676"]` |

> Se `allow_from` estiver vazio `[]`, qualquer numero pode falar com o bot.
> Os numeros devem ser informados sem o `+`, no formato E.164: codigo do pais + DDD + numero.

## Passo 4: Iniciar o Gateway

```bash
.\picoclaw.exe gateway
```

### Primeira execucao (login por QR Code)

Na primeira vez, o PicoClaw detecta que nao tem sessao salva e exibe um QR Code no terminal:

```
[INFO] [whatsapp] Starting WhatsApp channel (native whatsmeow)
[INFO] [whatsapp] No existing session found - starting QR code login

--- Scan this QR code with WhatsApp (Linked Devices) ---
  (QR Code aparece aqui)
--- Waiting for scan... ---
```

### Execucoes seguintes (sessao persistida)

A partir da segunda vez, conecta automaticamente sem QR:

```
[INFO] [whatsapp] Resuming existing session {device_id: ...}
[INFO] [whatsapp] WhatsApp connected
```

## Passo 5: Escanear o QR Code

No celular:

1. Abra o **WhatsApp**
2. Va em **Configuracoes** > **Dispositivos vinculados** > **Vincular dispositivo**
3. Escaneie o QR Code que apareceu no terminal
4. Aguarde a confirmacao no terminal:

```
[INFO] [whatsapp] WhatsApp login successful {device_id: ...}
[INFO] [whatsapp] WhatsApp channel started successfully
[INFO] [whatsapp] WhatsApp connected
```

> O QR Code expira em ~60 segundos. Se expirar, um novo eh gerado automaticamente.

## Passo 6: Testar

### Teste de texto

Envie uma mensagem pelo WhatsApp para o numero vinculado:

```
Ola, quem e voce?
```

No terminal do PicoClaw aparece:

```
[INFO] [whatsapp] Message received {sender: 5511982650676, chat: 5511982650676@s.whatsapp.net, preview: Ola, quem e voce?}
```

O agente processa e responde automaticamente no WhatsApp.

### Teste de imagem

Envie uma foto pelo WhatsApp. O arquivo eh baixado para:

```
%TEMP%\picoclaw_media\wa_1707696000000.jpg
```

### Teste de audio de voz (PTT)

Envie uma mensagem de voz (segure o microfone). Se o Groq estiver configurado:

```
[INFO] [whatsapp] Voice transcribed {text: Ola, tudo bem?}
```

Sem Groq, aparece como `[voice: caminho/do/arquivo.ogg]`.

### Teste de documento

Envie um PDF ou qualquer arquivo. Sera baixado com a extensao original.

### Teste de grupo

Adicione o numero vinculado a um grupo e envie uma mensagem. No log:

```
[INFO] [whatsapp] Message received {sender: 5511..., chat: 120363...@g.us, is_group: true}
```

## Passo 7: Parar o Gateway

Pressione `Ctrl+C` no terminal:

```
[INFO] [whatsapp] Stopping WhatsApp channel
[INFO] [whatsapp] WhatsApp channel stopped
```

## Verificar status

```bash
.\picoclaw.exe status
```

---

## Tabela de testes de validacao

| Teste | Como | Resultado esperado |
|-------|------|--------------------|
| Build | `go build ./...` | Sem erros |
| QR Login | Primeira execucao do gateway | QR aparece no terminal |
| Persistencia | Reiniciar gateway | Conecta sem QR |
| Texto | Enviar "oi" no WhatsApp | Agente responde |
| Imagem | Enviar foto | Arquivo em `%TEMP%\picoclaw_media\` |
| Voz PTT | Enviar audio de voz | Transcricao (se Groq configurado) |
| AllowList | Mensagem de numero nao listado | Ignorada silenciosamente |
| Grupo | Mensagem em grupo | `is_group=true` no log |
| Reconexao | Desligar/ligar Wi-Fi | Log de reconnect + reconecta |
| Shutdown | Ctrl+C | Saida limpa, sem panic |

---

## Troubleshooting

### QR nao aparece

- Verifique se `whatsapp.enabled` esta `true` no config.json

### "No API key configured"

- Configure pelo menos um provider em `providers` (recomendamos `openrouter`)

### Mensagem enviada mas sem resposta

- Seu numero esta no `allow_from`?
- O provider tem API key valida?
- Verifique os logs do terminal para erros

### "WhatsApp logged out"

- O dispositivo foi desvinculado no celular
- Delete `~/.picoclaw/whatsapp.db` e refaca o login QR

### Sessao corrompida

- Delete `C:\Users\Administrator\.picoclaw\whatsapp.db`
- Reinicie o gateway para gerar novo QR

### Reconexao automatica

O PicoClaw tenta reconectar automaticamente com backoff exponencial:
- 5s, 10s, 20s, 40s, ... ate no maximo 5 minutos entre tentativas
- Se o WhatsApp deslogar no servidor (dispositivo desvinculado), a reconexao para e voce precisa refazer o QR

---

## Configuracao com transcricao de voz (opcional)

Para habilitar transcricao automatica de audios de voz, adicione a API key do Groq:

```json
{
  "providers": {
    "groq": {
      "api_key": "gsk_SUA_CHAVE_GROQ"
    }
  }
}
```

O PicoClaw usa o modelo `whisper-large-v3` do Groq para transcrever audios PTT recebidos pelo WhatsApp. Obtenha uma chave em: https://console.groq.com/keys

---

## Arquitetura

```
WhatsApp Web API
    |
    v
whatsmeow.Client (conexao direta via WebSocket)
    |
    v
WhatsAppChannel.eventHandler()
    |
    v
handleIncomingMessage()
    |  - extrai texto, midia, mencoes
    |  - baixa midia para disco
    |  - transcreve voz (se Groq disponivel)
    v
BaseChannel.HandleMessage()
    |  - verifica allow_from
    |  - cria session key
    v
MessageBus.PublishInbound()
    |
    v
AgentLoop (processamento IA)
    |
    v
MessageBus.PublishOutbound()
    |
    v
WhatsAppChannel.Send()
    |  - envia indicador "digitando..."
    |  - envia mensagem
    |  - limpa indicador
    v
WhatsApp Web API -> Usuario recebe resposta
```

---

*Documento criado em 2026-02-12 como parte da feature/whatsapp-native.*
