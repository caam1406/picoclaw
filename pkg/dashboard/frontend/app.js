// PicoClaw Dashboard - Vanilla JS

let token = localStorage.getItem('picoclaw_token') || '';
let ws = null;
let currentContact = null; // {channel, id} of contact being edited
let currentDefault = null; // channel key of default being edited
let currentConfig = null;
let currentSecrets = {};
let liveMessages = [];
let qrState = null;
let waSelfJid = ''; // logged-in WhatsApp number (JID) when channel is connected, for "add my number"
const MAX_LIVE_MESSAGES = 200;

// ─── Init ───────────────────────────────────────────────────────────
window.addEventListener('DOMContentLoaded', () => {
  if (token) {
    enterDashboard();
  }
});

// ─── Auth ───────────────────────────────────────────────────────────
function doLogin() {
  const input = document.getElementById('token-input');
  token = input.value.trim();
  if (!token) return;

  // Test token with a simple API call
  apiFetch('/api/v1/status').then(data => {
    if (data.error) {
      document.getElementById('login-error').style.display = 'block';
      return;
    }
    localStorage.setItem('picoclaw_token', token);
    enterDashboard();
  }).catch(() => {
    document.getElementById('login-error').style.display = 'block';
  });
}

function doLogout() {
  token = '';
  localStorage.removeItem('picoclaw_token');
  if (ws) ws.close();
  document.getElementById('dashboard').style.display = 'none';
  document.getElementById('login-screen').style.display = 'flex';
}

function enterDashboard() {
  document.getElementById('login-screen').style.display = 'none';
  document.getElementById('dashboard').style.display = 'grid';
  connectWebSocket();
  loadOverview();
  loadChannels();
  loadContacts();
  loadDefaults();
  checkPendingQR();
  checkSetupStatus();
}

// ─── API Helper ─────────────────────────────────────────────────────
function apiFetch(path, opts = {}) {
  const url = path;
  const headers = { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' };
  return fetch(url, { ...opts, headers }).then(r => {
    if (r.status === 401) {
      doLogout();
      throw new Error('unauthorized');
    }
    return r.json();
  });
}

// ─── WebSocket ──────────────────────────────────────────────────────
function connectWebSocket() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = proto + '//' + location.host + '/ws?token=' + encodeURIComponent(token);

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    document.getElementById('ws-dot').classList.add('connected');
    document.getElementById('ws-label').textContent = 'Conectado';
  };

  ws.onclose = () => {
    document.getElementById('ws-dot').classList.remove('connected');
    document.getElementById('ws-label').textContent = 'Desconectado';
    // Reconnect after 3s
    setTimeout(connectWebSocket, 3000);
  };

  ws.onerror = () => {};

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      handleBusEvent(data);
    } catch (e) {}
  };
}

function handleBusEvent(event) {
  // Handle QR code events
  if (event.type === 'qr_code' && event.qr_code) {
    handleQREvent(event.qr_code);
    return;
  }

  const time = new Date(event.time).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  let channel = '', sender = '', content = '';

  if (event.type === 'inbound' && event.inbound) {
    channel = event.inbound.channel;
    sender = event.inbound.sender_id;
    content = event.inbound.content;
  } else if (event.type === 'outbound' && event.outbound) {
    channel = event.outbound.channel;
    content = event.outbound.content;
  }

  if (!content) return;

  liveMessages.unshift({ time, type: event.type, channel, sender, content });
  if (liveMessages.length > MAX_LIVE_MESSAGES) liveMessages.pop();

  renderLiveMessages();
}

function renderLiveMessages() {
  const container = document.getElementById('live-messages');
  if (liveMessages.length === 0) {
    container.innerHTML = '<div style="color:var(--text-muted);padding:20px;text-align:center">Aguardando mensagens...</div>';
    return;
  }

  container.innerHTML = liveMessages.map(m => {
    const dirClass = m.type === 'inbound' ? 'inbound' : 'outbound';
    const dirLabel = m.type === 'inbound' ? 'IN' : 'OUT';
    const preview = m.content.length > 200 ? m.content.slice(0, 200) + '...' : m.content;
    return `<div class="msg-item">
      <span class="msg-time">${m.time}</span>
      <span class="msg-direction ${dirClass}">${dirLabel}</span>
      <span class="msg-channel">${m.channel}</span>
      <span class="msg-content">${escapeHtml(preview)}</span>
    </div>`;
  }).join('');
}

// ─── Load Data ──────────────────────────────────────────────────────
function loadOverview() {
  Promise.all([
    apiFetch('/api/v1/status'),
    apiFetch('/api/v1/sessions'),
    apiFetch('/api/v1/contacts')
  ]).then(([status, sessions, contacts]) => {
    const grid = document.getElementById('stats-grid');
    const channelCount = status.channels ? Object.keys(status.channels).length : 0;
    const runningCount = status.channels ? Object.values(status.channels).filter(c => c.running).length : 0;
    const sessionCount = sessions ? sessions.length : 0;
    const contactCount = contacts ? contacts.length : 0;

    grid.innerHTML = `
      <div class="stat-box"><div class="stat-value">${runningCount}/${channelCount}</div><div class="stat-label">Canais Ativos</div></div>
      <div class="stat-box"><div class="stat-value">${sessionCount}</div><div class="stat-label">Sessoes</div></div>
      <div class="stat-box"><div class="stat-value">${contactCount}</div><div class="stat-label">Contatos</div></div>
      <div class="stat-box"><div class="stat-value">${status.uptime || '-'}</div><div class="stat-label">Uptime</div></div>
    `;

    // Sessions list
    const sessList = document.getElementById('sessions-list');
    if (!sessions || sessions.length === 0) {
      sessList.innerHTML = '<div style="color:var(--text-muted);padding:12px">Nenhuma sessao ativa</div>';
      return;
    }

    // Sort by updated desc
    sessions.sort((a, b) => new Date(b.updated) - new Date(a.updated));

    sessList.innerHTML = sessions.slice(0, 20).map(s => {
      const updated = new Date(s.updated).toLocaleString('pt-BR');
      return `<div class="sidebar-item" style="margin:0;cursor:default">
        <span style="flex:1;font-family:var(--mono);font-size:12px">${escapeHtml(s.key)}</span>
        <span style="color:var(--text-muted);font-size:11px">${s.message_count} msgs</span>
        <span style="color:var(--text-muted);font-size:11px">${updated}</span>
      </div>`;
    }).join('');
  }).catch(() => {});
}

function loadChannels() {
  apiFetch('/api/v1/channels').then(data => {
    const list = document.getElementById('channels-list');
    if (!data || Object.keys(data).length === 0) {
      list.innerHTML = '<div style="color:var(--text-muted);padding:8px 12px;font-size:13px">Nenhum canal</div>';
      updateSidebarQR('disconnected');
      return;
    }

    // Update sidebar WhatsApp connection status
    const wa = data['whatsapp'];
    if (wa) {
      updateSidebarQR(wa.running ? 'connected' : (qrState ? 'pending' : 'disconnected'));
    } else {
      updateSidebarQR('disconnected');
    }

    if (data.whatsapp && data.whatsapp.self_jid) {
      waSelfJid = data.whatsapp.self_jid;
    } else {
      waSelfJid = '';
    }

    list.innerHTML = Object.entries(data).map(([name, info]) => {
      const dotClass = info.running ? 'running' : '';
      const clickAction = name === 'whatsapp' && !info.running ? `onclick="showView('qr')"` : '';
      const statusLabel = info.running ? 'ativo' : (name === 'whatsapp' ? 'aguardando' : 'parado');
      return `<div class="sidebar-item" ${clickAction}>
        <span class="channel-dot ${dotClass}"></span>
        <span>${capitalize(name)}</span>
        <span style="margin-left:auto;font-size:11px;color:var(--text-muted)">${statusLabel}</span>
      </div>`;
    }).join('');
  }).catch(() => {});
}

function loadContacts() {
  apiFetch('/api/v1/contacts').then(data => {
    const list = document.getElementById('contacts-list');
    if (!data || data.length === 0) {
      list.innerHTML = '<div style="color:var(--text-muted);padding:8px 12px;font-size:13px">Nenhum contato</div>';
      return;
    }

    list.innerHTML = data.map(c => {
      const label = c.display_name || c.contact_id;
      const isGroup = (c.contact_id && c.contact_id.includes('@g.us'));
      const groupTag = isGroup ? '<span class="contact-tag group-tag">Grupo</span>' : '';
      return `<div class="sidebar-item" onclick="editContact('${escapeAttr(c.channel)}','${escapeAttr(c.contact_id)}')">
        <span>${escapeHtml(label)}</span>
        ${groupTag}
        <span class="contact-tag">${c.channel}</span>
      </div>`;
    }).join('');
  }).catch(() => {});
}

// ─── Views ──────────────────────────────────────────────────────────
function showView(name) {
  document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
  const view = document.getElementById('view-' + name);
  if (view) view.classList.add('active');

  if (name === 'overview') loadOverview();
  if (name === 'settings') loadSettings();
  if (name === 'defaults') loadDefaults();
}

// ─── Contact CRUD ───────────────────────────────────────────────────
function editContact(channel, id) {
  currentContact = { channel, id };

  apiFetch('/api/v1/contacts/' + encodeURIComponent(channel) + '/' + encodeURIComponent(id)).then(data => {
    document.getElementById('contact-channel').value = data.channel;
    document.getElementById('contact-id').value = data.contact_id;
    document.getElementById('contact-name').value = data.display_name || '';
    document.getElementById('contact-instructions').value = data.instructions || '';
    const isGroup = (data.contact_id && data.contact_id.includes('@g.us'));
    document.getElementById('contact-title').textContent = data.display_name || data.contact_id;
    document.getElementById('contact-subtitle').textContent = data.channel + ' / ' + data.contact_id + (isGroup ? ' (grupo)' : '');
    document.getElementById('contact-avatar').textContent = (data.display_name || data.contact_id).charAt(0).toUpperCase();
    document.getElementById('btn-delete-contact').style.display = 'inline-block';

    // Disable channel/id fields when editing existing
    document.getElementById('contact-channel').disabled = true;
    document.getElementById('contact-id').disabled = true;

    showView('contact');
  }).catch(() => {
    toast('Erro ao carregar contato', true);
  });
}

let waContactListData = null; // { self_jid, groups, address_book_contacts, recent_chats }

function showAddContact() {
  document.getElementById('modal-add-contact').classList.add('active');
  document.getElementById('new-contact-channel').value = 'whatsapp';
  document.getElementById('new-contact-id').value = '';
  document.getElementById('new-contact-name').value = '';
  onAddContactChannelChange();
}

function onAddContactChannelChange() {
  const channel = document.getElementById('new-contact-channel').value;
  const container = document.getElementById('add-contact-wa-list-container');
  const selfJidHint = document.getElementById('add-contact-self-jid-hint');
  const selfJidBtn = document.getElementById('btn-use-wa-self');
  if (channel === 'whatsapp') {
    container.style.display = 'block';
    loadWhatsAppContactList();
    if (waSelfJid) {
      if (selfJidHint) selfJidHint.textContent = 'Numero conectado: ' + waSelfJid.replace(/@.*/, '');
      if (selfJidHint) selfJidHint.style.display = 'block';
      if (selfJidBtn) selfJidBtn.style.display = 'inline-block';
    } else {
      if (selfJidHint) selfJidHint.style.display = 'none';
      if (selfJidBtn) selfJidBtn.style.display = 'none';
    }
  } else {
    container.style.display = 'none';
    if (selfJidHint) selfJidHint.style.display = 'none';
    if (selfJidBtn) selfJidBtn.style.display = 'none';
  }
}

function loadWhatsAppContactList() {
  const listEl = document.getElementById('add-contact-wa-list');
  if (!listEl) return;
  listEl.innerHTML = '<div style="padding:16px;color:var(--text-muted);text-align:center">Carregando...</div>';
  apiFetch('/api/v1/whatsapp/contact-list').then(data => {
    waContactListData = data;
    const html = [];
    if (data.error) {
      listEl.innerHTML = '<div class="form-hint" style="padding:12px">WhatsApp nao conectado ou canal indisponivel.</div>';
      return;
    }
    if (data.self_jid) {
      html.push('<div class="wa-contact-list-section">Meu numero</div>');
      html.push(buildWaContactItem(data.self_jid, 'Meu numero', data.self_jid.replace(/@.*/, ''), 'self'));
    }
    if (data.address_book_contacts && data.address_book_contacts.length > 0) {
      html.push('<div class="wa-contact-list-section">Agenda (' + data.address_book_contacts.length + ')</div>');
      data.address_book_contacts.forEach(c => {
        html.push(buildWaContactItem(c.jid, c.label || c.jid, c.jid, 'Contato'));
      });
    }
    if (data.recent_chats && data.recent_chats.length > 0) {
      const contactsOnly = data.recent_chats.filter(c => !c.is_group);
      if (contactsOnly.length > 0) {
        html.push('<div class="wa-contact-list-section">Contatos (' + contactsOnly.length + ')</div>');
        contactsOnly.forEach(c => {
          const label = c.label || c.jid;
          const meta = c.jid;
          html.push(buildWaContactItem(c.jid, label, meta, 'Contato'));
        });
      }
    }
    if (data.groups && data.groups.length > 0) {
      html.push('<div class="wa-contact-list-section">Grupos (' + data.groups.length + ')</div>');
      data.groups.forEach(g => {
        html.push(buildWaContactItem(g.jid, g.name, g.participant_count + ' participantes', 'Grupo'));
      });
    }
    if (data.recent_chats && data.recent_chats.length > 0) {
      const groupsInRecent = data.recent_chats.filter(c => c.is_group);
      if (groupsInRecent.length > 0) {
        html.push('<div class="wa-contact-list-section">Outros chats recentes</div>');
        groupsInRecent.forEach(c => {
          const label = c.label || c.jid;
          html.push(buildWaContactItem(c.jid, label, c.jid, 'Grupo'));
        });
      }
    }
    if (html.length === 0) {
      listEl.innerHTML = '<div class="form-hint" style="padding:12px">Nenhum contato ou grupo ainda. Envie uma mensagem no WhatsApp (ou receba de alguem) para o contato aparecer aqui.</div>';
    } else {
      listEl.innerHTML = html.join('');
    }
  }).catch(() => {
    listEl.innerHTML = '<div class="form-hint" style="padding:12px">Erro ao carregar lista.</div>';
  });
}

function attrEscape(s) {
  return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
}
function buildWaContactItem(jid, name, meta, tag) {
  const safeName = escapeHtml(name);
  const safeMeta = escapeHtml(meta);
  const safeTag = escapeHtml(tag);
  const dataJid = attrEscape(jid);
  const dataName = attrEscape(name);
  return '<div class="wa-contact-item" data-jid="' + dataJid + '" data-name="' + dataName + '" onclick="prefillContactFromWaEl(this)" title="Clique para preencher">' +
    '<div class="wa-contact-info"><div class="wa-contact-name">' + safeName + '</div><div class="wa-contact-meta">' + safeMeta + '</div></div>' +
    '<span class="wa-contact-tag">' + safeTag + '</span>' +
    '<button type="button" class="btn btn-primary btn-use-contact" onclick="event.stopPropagation(); prefillContactFromWaEl(this.parentElement); createContact();">Adicionar</button>' +
    '</div>';
}

function prefillContactFromWaEl(el) {
  const jid = el.getAttribute('data-jid');
  const name = el.getAttribute('data-name');
  if (jid) prefillContactFromWa(jid, name || jid);
}

function prefillContactFromWa(jid, displayName) {
  document.getElementById('new-contact-channel').value = 'whatsapp';
  document.getElementById('new-contact-id').value = jid;
  document.getElementById('new-contact-name').value = displayName || jid;
}

function useWhatsAppSelfAsContact() {
  if (!waSelfJid) return;
  document.getElementById('new-contact-channel').value = 'whatsapp';
  document.getElementById('new-contact-id').value = waSelfJid;
  document.getElementById('new-contact-name').value = 'Meu numero';
}

function closeModal() {
  document.getElementById('modal-add-contact').classList.remove('active');
}

function createContact() {
  const channel = document.getElementById('new-contact-channel').value;
  const id = document.getElementById('new-contact-id').value.trim();
  const name = document.getElementById('new-contact-name').value.trim();

  if (!id) {
    toast('ID do contato obrigatorio', true);
    return;
  }

  apiFetch('/api/v1/contacts/' + encodeURIComponent(channel) + '/' + encodeURIComponent(id), {
    method: 'PUT',
    body: JSON.stringify({ display_name: name, instructions: '' })
  }).then(() => {
    closeModal();
    loadContacts();
    toast('Contato criado');
    // Open edit view for the new contact
    currentContact = { channel, id };
    document.getElementById('contact-channel').value = channel;
    document.getElementById('contact-channel').disabled = true;
    document.getElementById('contact-id').value = id;
    document.getElementById('contact-id').disabled = true;
    document.getElementById('contact-name').value = name;
    document.getElementById('contact-instructions').value = '';
    const isGroup = id.includes('@g.us');
    document.getElementById('contact-title').textContent = name || id;
    document.getElementById('contact-subtitle').textContent = channel + ' / ' + id + (isGroup ? ' (grupo)' : '');
    document.getElementById('contact-avatar').textContent = (name || id).charAt(0).toUpperCase();
    document.getElementById('btn-delete-contact').style.display = 'inline-block';
    showView('contact');
  }).catch(() => {
    toast('Erro ao criar contato', true);
  });
}

function saveContact() {
  if (!currentContact) return;

  const name = document.getElementById('contact-name').value.trim();
  const instructions = document.getElementById('contact-instructions').value.trim();

  apiFetch('/api/v1/contacts/' + encodeURIComponent(currentContact.channel) + '/' + encodeURIComponent(currentContact.id), {
    method: 'PUT',
    body: JSON.stringify({ display_name: name, instructions: instructions })
  }).then(() => {
    toast('Instrucoes salvas');
    loadContacts();
  }).catch(() => {
    toast('Erro ao salvar', true);
  });
}

function deleteContact() {
  if (!currentContact) return;
  if (!confirm('Excluir instrucoes para este contato?')) return;

  apiFetch('/api/v1/contacts/' + encodeURIComponent(currentContact.channel) + '/' + encodeURIComponent(currentContact.id), {
    method: 'DELETE'
  }).then(() => {
    toast('Contato removido');
    loadContacts();
    showView('overview');
    currentContact = null;
  }).catch(() => {
    toast('Erro ao excluir', true);
  });
}

// ─── Default Instructions CRUD ───────────────────────────────────────
function getDefaultLabel(channel) {
  if (channel === '*') return 'Global';
  return capitalize(channel);
}

function loadDefaults() {
  apiFetch('/api/v1/defaults').then(data => {
    const list = document.getElementById('defaults-list');
    if (!data || Object.keys(data).length === 0) {
      list.innerHTML = '<div style="color:var(--text-muted);padding:8px 12px;font-size:13px">Nenhuma instrucao</div>';
      return;
    }

    list.innerHTML = Object.entries(data).map(([channel, inst]) => {
      const label = getDefaultLabel(channel);
      const preview = inst.length > 30 ? inst.slice(0, 30) + '...' : inst;
      return `<div class="sidebar-item" onclick="editDefault('${escapeAttr(channel)}')">
        <span>${escapeHtml(label)}</span>
        <span class="contact-tag">${channel === '*' ? 'global' : channel}</span>
      </div>`;
    }).join('');
  }).catch(() => {});
}

function editDefault(channel) {
  currentDefault = channel;

  apiFetch('/api/v1/defaults/' + encodeURIComponent(channel)).then(data => {
    document.getElementById('default-channel').value = data.channel;
    document.getElementById('default-channel').disabled = true;
    document.getElementById('default-instructions').value = data.instructions || '';
    document.getElementById('default-title').textContent = 'Instrucao: ' + getDefaultLabel(data.channel);
    document.getElementById('default-subtitle').textContent = data.channel === '*' ? 'Aplica-se a todos os nao-contatos' : 'Aplica-se a nao-contatos no ' + capitalize(data.channel);
    document.getElementById('default-avatar').textContent = data.channel === '*' ? '*' : data.channel.charAt(0).toUpperCase();
    document.getElementById('btn-delete-default').style.display = 'inline-block';

    showView('default');
  }).catch(() => {
    toast('Erro ao carregar instrucao padrao', true);
  });
}

function showAddDefault() {
  document.getElementById('modal-add-default').classList.add('active');
  document.getElementById('new-default-channel').value = '*';
}

function closeDefaultModal() {
  document.getElementById('modal-add-default').classList.remove('active');
}

function createDefault() {
  const channel = document.getElementById('new-default-channel').value;

  apiFetch('/api/v1/defaults/' + encodeURIComponent(channel), {
    method: 'PUT',
    body: JSON.stringify({ instructions: '' })
  }).then(() => {
    closeDefaultModal();
    loadDefaults();
    toast('Instrucao padrao criada');
    // Open edit view
    currentDefault = channel;
    document.getElementById('default-channel').value = channel;
    document.getElementById('default-channel').disabled = true;
    document.getElementById('default-instructions').value = '';
    document.getElementById('default-title').textContent = 'Instrucao: ' + getDefaultLabel(channel);
    document.getElementById('default-subtitle').textContent = channel === '*' ? 'Aplica-se a todos os nao-contatos' : 'Aplica-se a nao-contatos no ' + capitalize(channel);
    document.getElementById('default-avatar').textContent = channel === '*' ? '*' : channel.charAt(0).toUpperCase();
    document.getElementById('btn-delete-default').style.display = 'inline-block';
    showView('default');
  }).catch(() => {
    toast('Erro ao criar instrucao padrao', true);
  });
}

function saveDefault() {
  if (!currentDefault) return;

  const instructions = document.getElementById('default-instructions').value.trim();

  apiFetch('/api/v1/defaults/' + encodeURIComponent(currentDefault), {
    method: 'PUT',
    body: JSON.stringify({ instructions: instructions })
  }).then(() => {
    toast('Instrucao padrao salva');
    loadDefaults();
  }).catch(() => {
    toast('Erro ao salvar', true);
  });
}

function deleteDefault() {
  if (!currentDefault) return;
  if (!confirm('Excluir instrucao padrao para ' + getDefaultLabel(currentDefault) + '?')) return;

  apiFetch('/api/v1/defaults/' + encodeURIComponent(currentDefault), {
    method: 'DELETE'
  }).then(() => {
    toast('Instrucao padrao removida');
    loadDefaults();
    showView('overview');
    currentDefault = null;
  }).catch(() => {
    toast('Erro ao excluir', true);
  });
}

// ─── Toast ──────────────────────────────────────────────────────────
function loadSettings() {
  loadStorageConfig();
  loadAppConfig();
}

function loadAppConfig() {
  apiFetch('/api/v1/config').then(data => {
    currentConfig = data.config || {};
    currentSecrets = data.secrets || {};
    applyConfigToForm(currentConfig);
  }).catch(() => {
    toast('Erro ao carregar configuracao', true);
  });
}

function applyConfigToForm(cfg) {
  const agents = cfg.agents || {};
  const defaults = agents.defaults || {};
  setValue('agent-workspace', defaults.workspace || '');
  setValue('agent-model', defaults.model || '');
  setValue('agent-max-tokens', defaults.max_tokens ?? '');
  setValue('agent-temperature', defaults.temperature ?? '');
  setValue('agent-max-tool-iterations', defaults.max_tool_iterations ?? '');

  const gateway = cfg.gateway || {};
  setValue('gateway-host', gateway.host || '');
  setValue('gateway-port', gateway.port ?? '');

  const dashboard = cfg.dashboard || {};
  setChecked('dashboard-enabled', !!dashboard.enabled);
  setValue('dashboard-host', dashboard.host || '');
  setValue('dashboard-port', dashboard.port ?? '');
  setChecked('dashboard-contacts-only', !!dashboard.contacts_only);
  setSecretHint('dashboard-token-hint', 'dashboard.token');

  const providers = cfg.providers || {};
  setValue('provider-openrouter-base', (providers.openrouter || {}).api_base || '');
  setValue('provider-anthropic-base', (providers.anthropic || {}).api_base || '');
  setValue('provider-openai-base', (providers.openai || {}).api_base || '');
  setValue('provider-gemini-base', (providers.gemini || {}).api_base || '');
  setValue('provider-zhipu-base', (providers.zhipu || {}).api_base || '');
  setValue('provider-zai-base', (providers.zai || {}).api_base || '');
  setValue('provider-groq-base', (providers.groq || {}).api_base || '');
  setValue('provider-vllm-base', (providers.vllm || {}).api_base || '');

  setSecretHint('provider-openrouter-key-hint', 'providers.openrouter.api_key');
  setSecretHint('provider-anthropic-key-hint', 'providers.anthropic.api_key');
  setSecretHint('provider-openai-key-hint', 'providers.openai.api_key');
  setSecretHint('provider-gemini-key-hint', 'providers.gemini.api_key');
  setSecretHint('provider-zhipu-key-hint', 'providers.zhipu.api_key');
  setSecretHint('provider-zai-key-hint', 'providers.zai.api_key');
  setSecretHint('provider-groq-key-hint', 'providers.groq.api_key');
  setSecretHint('provider-vllm-key-hint', 'providers.vllm.api_key');

  const channels = cfg.channels || {};
  const whatsapp = channels.whatsapp || {};
  setChecked('channel-whatsapp-enabled', !!whatsapp.enabled);
  setValue('channel-whatsapp-store-path', whatsapp.store_path || '');
  setValue('channel-whatsapp-allow', joinList(whatsapp.allow_from));

  const telegram = channels.telegram || {};
  setChecked('channel-telegram-enabled', !!telegram.enabled);
  setValue('channel-telegram-allow', joinList(telegram.allow_from));
  setSecretHint('channel-telegram-token-hint', 'channels.telegram.token');

  const discord = channels.discord || {};
  setChecked('channel-discord-enabled', !!discord.enabled);
  setValue('channel-discord-allow', joinList(discord.allow_from));
  setSecretHint('channel-discord-token-hint', 'channels.discord.token');

  const feishu = channels.feishu || {};
  setChecked('channel-feishu-enabled', !!feishu.enabled);
  setValue('channel-feishu-app-id', feishu.app_id || '');
  setValue('channel-feishu-allow', joinList(feishu.allow_from));
  setSecretHint('channel-feishu-app-secret-hint', 'channels.feishu.app_secret');
  setSecretHint('channel-feishu-encrypt-key-hint', 'channels.feishu.encrypt_key');
  setSecretHint('channel-feishu-verification-token-hint', 'channels.feishu.verification_token');

  const qq = channels.qq || {};
  setChecked('channel-qq-enabled', !!qq.enabled);
  setValue('channel-qq-app-id', qq.app_id || '');
  setValue('channel-qq-allow', joinList(qq.allow_from));
  setSecretHint('channel-qq-app-secret-hint', 'channels.qq.app_secret');

  const dingtalk = channels.dingtalk || {};
  setChecked('channel-dingtalk-enabled', !!dingtalk.enabled);
  setValue('channel-dingtalk-client-id', dingtalk.client_id || '');
  setValue('channel-dingtalk-allow', joinList(dingtalk.allow_from));
  setSecretHint('channel-dingtalk-client-secret-hint', 'channels.dingtalk.client_secret');

  const maixcam = channels.maixcam || {};
  setChecked('channel-maixcam-enabled', !!maixcam.enabled);
  setValue('channel-maixcam-host', maixcam.host || '');
  setValue('channel-maixcam-port', maixcam.port ?? '');
  setValue('channel-maixcam-allow', joinList(maixcam.allow_from));

  const tools = cfg.tools || {};
  const webSearch = (tools.web || {}).search || {};
  setValue('tools-web-search-max', webSearch.max_results ?? '');
  setSecretHint('tools-web-search-key-hint', 'tools.web.search.api_key');

  clearSecretInputs([
    'dashboard-token',
    'provider-openrouter-key',
    'provider-anthropic-key',
    'provider-openai-key',
    'provider-gemini-key',
    'provider-zhipu-key',
    'provider-zai-key',
    'provider-groq-key',
    'provider-vllm-key',
    'channel-telegram-token',
    'channel-discord-token',
    'channel-feishu-app-secret',
    'channel-feishu-encrypt-key',
    'channel-feishu-verification-token',
    'channel-qq-app-secret',
    'channel-dingtalk-client-secret',
    'tools-web-search-key'
  ]);
}

function saveAppConfig() {
  if (!currentConfig) {
    toast('Configuracao nao carregada', true);
    return;
  }

  const cfg = JSON.parse(JSON.stringify(currentConfig));
  cfg.agents = cfg.agents || {};
  cfg.agents.defaults = cfg.agents.defaults || {};
  cfg.agents.defaults.workspace = getValue('agent-workspace');
  cfg.agents.defaults.model = getValue('agent-model');
  cfg.agents.defaults.max_tokens = toInt(getValue('agent-max-tokens'), cfg.agents.defaults.max_tokens);
  cfg.agents.defaults.temperature = toFloat(getValue('agent-temperature'), cfg.agents.defaults.temperature);
  cfg.agents.defaults.max_tool_iterations = toInt(getValue('agent-max-tool-iterations'), cfg.agents.defaults.max_tool_iterations);

  cfg.gateway = cfg.gateway || {};
  cfg.gateway.host = getValue('gateway-host');
  cfg.gateway.port = toInt(getValue('gateway-port'), cfg.gateway.port);

  cfg.dashboard = cfg.dashboard || {};
  cfg.dashboard.enabled = getChecked('dashboard-enabled');
  cfg.dashboard.host = getValue('dashboard-host');
  cfg.dashboard.port = toInt(getValue('dashboard-port'), cfg.dashboard.port);
  cfg.dashboard.contacts_only = getChecked('dashboard-contacts-only');

  cfg.providers = cfg.providers || {};
  cfg.providers.openrouter = cfg.providers.openrouter || {};
  cfg.providers.openrouter.api_base = getValue('provider-openrouter-base');
  cfg.providers.anthropic = cfg.providers.anthropic || {};
  cfg.providers.anthropic.api_base = getValue('provider-anthropic-base');
  cfg.providers.openai = cfg.providers.openai || {};
  cfg.providers.openai.api_base = getValue('provider-openai-base');
  cfg.providers.gemini = cfg.providers.gemini || {};
  cfg.providers.gemini.api_base = getValue('provider-gemini-base');
  cfg.providers.zhipu = cfg.providers.zhipu || {};
  cfg.providers.zhipu.api_base = getValue('provider-zhipu-base');
  cfg.providers.zai = cfg.providers.zai || {};
  cfg.providers.zai.api_base = getValue('provider-zai-base');
  cfg.providers.groq = cfg.providers.groq || {};
  cfg.providers.groq.api_base = getValue('provider-groq-base');
  cfg.providers.vllm = cfg.providers.vllm || {};
  cfg.providers.vllm.api_base = getValue('provider-vllm-base');

  cfg.channels = cfg.channels || {};
  cfg.channels.whatsapp = cfg.channels.whatsapp || {};
  cfg.channels.whatsapp.enabled = getChecked('channel-whatsapp-enabled');
  cfg.channels.whatsapp.store_path = getValue('channel-whatsapp-store-path');
  cfg.channels.whatsapp.allow_from = parseList(getValue('channel-whatsapp-allow'));

  cfg.channels.telegram = cfg.channels.telegram || {};
  cfg.channels.telegram.enabled = getChecked('channel-telegram-enabled');
  cfg.channels.telegram.allow_from = parseList(getValue('channel-telegram-allow'));

  cfg.channels.discord = cfg.channels.discord || {};
  cfg.channels.discord.enabled = getChecked('channel-discord-enabled');
  cfg.channels.discord.allow_from = parseList(getValue('channel-discord-allow'));

  cfg.channels.feishu = cfg.channels.feishu || {};
  cfg.channels.feishu.enabled = getChecked('channel-feishu-enabled');
  cfg.channels.feishu.app_id = getValue('channel-feishu-app-id');
  cfg.channels.feishu.allow_from = parseList(getValue('channel-feishu-allow'));

  cfg.channels.qq = cfg.channels.qq || {};
  cfg.channels.qq.enabled = getChecked('channel-qq-enabled');
  cfg.channels.qq.app_id = getValue('channel-qq-app-id');
  cfg.channels.qq.allow_from = parseList(getValue('channel-qq-allow'));

  cfg.channels.dingtalk = cfg.channels.dingtalk || {};
  cfg.channels.dingtalk.enabled = getChecked('channel-dingtalk-enabled');
  cfg.channels.dingtalk.client_id = getValue('channel-dingtalk-client-id');
  cfg.channels.dingtalk.allow_from = parseList(getValue('channel-dingtalk-allow'));

  cfg.channels.maixcam = cfg.channels.maixcam || {};
  cfg.channels.maixcam.enabled = getChecked('channel-maixcam-enabled');
  cfg.channels.maixcam.host = getValue('channel-maixcam-host');
  cfg.channels.maixcam.port = toInt(getValue('channel-maixcam-port'), cfg.channels.maixcam.port);
  cfg.channels.maixcam.allow_from = parseList(getValue('channel-maixcam-allow'));

  cfg.tools = cfg.tools || {};
  cfg.tools.web = cfg.tools.web || {};
  cfg.tools.web.search = cfg.tools.web.search || {};
  cfg.tools.web.search.max_results = toInt(getValue('tools-web-search-max'), cfg.tools.web.search.max_results);

  const secrets = {};
  collectSecret('dashboard-token', 'dashboard.token', secrets);
  collectSecret('provider-openrouter-key', 'providers.openrouter.api_key', secrets);
  collectSecret('provider-anthropic-key', 'providers.anthropic.api_key', secrets);
  collectSecret('provider-openai-key', 'providers.openai.api_key', secrets);
  collectSecret('provider-gemini-key', 'providers.gemini.api_key', secrets);
  collectSecret('provider-zhipu-key', 'providers.zhipu.api_key', secrets);
  collectSecret('provider-zai-key', 'providers.zai.api_key', secrets);
  collectSecret('provider-groq-key', 'providers.groq.api_key', secrets);
  collectSecret('provider-vllm-key', 'providers.vllm.api_key', secrets);
  collectSecret('channel-telegram-token', 'channels.telegram.token', secrets);
  collectSecret('channel-discord-token', 'channels.discord.token', secrets);
  collectSecret('channel-feishu-app-secret', 'channels.feishu.app_secret', secrets);
  collectSecret('channel-feishu-encrypt-key', 'channels.feishu.encrypt_key', secrets);
  collectSecret('channel-feishu-verification-token', 'channels.feishu.verification_token', secrets);
  collectSecret('channel-qq-app-secret', 'channels.qq.app_secret', secrets);
  collectSecret('channel-dingtalk-client-secret', 'channels.dingtalk.client_secret', secrets);
  collectSecret('tools-web-search-key', 'tools.web.search.api_key', secrets);

  apiFetch('/api/v1/config', {
    method: 'PUT',
    body: JSON.stringify({ config: cfg, secrets: secrets })
  }).then(() => {
    toast('Configuracao salva');
    loadAppConfig();
  }).catch(err => {
    toast('Erro ao salvar: ' + err.message, true);
  });
}

function toast(msg, isError) {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = 'toast' + (isError ? ' error' : '');
  el.classList.add('show');
  setTimeout(() => el.classList.remove('show'), 2500);
}

// ─── Helpers ────────────────────────────────────────────────────────
function setValue(id, value) {
  const el = document.getElementById(id);
  if (!el) return;
  el.value = value ?? '';
}

function getValue(id) {
  const el = document.getElementById(id);
  if (!el) return '';
  return el.value.trim();
}

function setChecked(id, value) {
  const el = document.getElementById(id);
  if (!el) return;
  el.checked = !!value;
}

function getChecked(id) {
  const el = document.getElementById(id);
  return el ? el.checked : false;
}

function setSecretHint(id, path) {
  const el = document.getElementById(id);
  if (!el) return;
  const masked = currentSecrets[path];
  el.textContent = masked ? 'Atual: ' + masked : 'Nao configurado';
}

function joinList(list) {
  if (!Array.isArray(list)) return '';
  return list.join(', ');
}

function parseList(value) {
  if (!value) return [];
  return value.split(/[,\\n]+/).map(v => v.trim()).filter(Boolean);
}

function toInt(value, fallback) {
  const n = parseInt(value, 10);
  return Number.isNaN(n) ? fallback : n;
}

function toFloat(value, fallback) {
  const n = parseFloat(value);
  return Number.isNaN(n) ? fallback : n;
}

function clearSecretInputs(ids) {
  ids.forEach(id => {
    const el = document.getElementById(id);
    if (el) {
      el.value = '';
    }
  });
}

function collectSecret(inputId, path, secrets) {
  const el = document.getElementById(inputId);
  if (!el) return;
  const value = el.value.trim();
  if (value) {
    secrets[path] = value;
  }
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function escapeAttr(str) {
  return str.replace(/'/g, "\\'").replace(/"/g, '&quot;');
}

function capitalize(str) {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

// ─── Storage Configuration ──────────────────────────────────────────
function loadStorageConfig() {
  apiFetch('/api/v1/config/storage').then(data => {
    document.getElementById('storage-type').value = data.type || 'file';
    document.getElementById('storage-url').value = data.database_url || '';
    document.getElementById('storage-filepath').value = data.file_path || '~/.picoclaw/workspace';
    document.getElementById('storage-ssl-enabled').checked = data.ssl_enabled || false;
    toggleStorageFields();
  }).catch(() => {
    toast('Erro ao carregar configuracao de storage', true);
  });
}

function toggleStorageFields() {
  const type = document.getElementById('storage-type').value;
  const urlGroup = document.getElementById('storage-url-group');
  const fileGroup = document.getElementById('storage-filepath-group');
  const sslGroup = document.getElementById('storage-ssl-group');

  if (type === 'file') {
    urlGroup.style.display = 'none';
    fileGroup.style.display = 'block';
    sslGroup.style.display = 'none';
  } else if (type === 'postgres' || type === 'sqlite') {
    urlGroup.style.display = 'block';
    fileGroup.style.display = 'none';
    sslGroup.style.display = 'block';
  }
}

function testStorageConnection() {
  const type = document.getElementById('storage-type').value;
  const url = document.getElementById('storage-url').value.trim();
  const filePath = document.getElementById('storage-filepath').value.trim();
  const sslEnabled = document.getElementById('storage-ssl-enabled').checked;

  const resultDiv = document.getElementById('storage-test-result');
  resultDiv.className = 'storage-result';
  resultDiv.textContent = 'Testando conexão...';
  resultDiv.classList.add('show');

  apiFetch('/api/v1/config/storage/test', {
    method: 'POST',
    body: JSON.stringify({
      type: type,
      database_url: url,
      file_path: filePath,
      ssl_enabled: sslEnabled
    })
  }).then(data => {
    if (data.success) {
      resultDiv.className = 'storage-result success show';
      resultDiv.textContent = '✓ Conexão bem-sucedida! Storage está acessível.';
    } else {
      resultDiv.className = 'storage-result error show';
      resultDiv.textContent = '✗ Erro: ' + (data.error || 'Falha ao conectar');
    }
  }).catch(err => {
    resultDiv.className = 'storage-result error show';
    resultDiv.textContent = '✗ Erro ao testar conexão: ' + err.message;
  });
}

function saveStorageConfig() {
  const type = document.getElementById('storage-type').value;
  const url = document.getElementById('storage-url').value.trim();
  const filePath = document.getElementById('storage-filepath').value.trim();
  const sslEnabled = document.getElementById('storage-ssl-enabled').checked;

  if (type === 'sqlite') {
    toast('SQLite ainda não está implementado', true);
    return;
  }

  if (type === 'postgres' && !url) {
    toast('Database URL é obrigatória para PostgreSQL', true);
    return;
  }

  if (type === 'file' && !filePath) {
    toast('Caminho do workspace é obrigatório para file-based storage', true);
    return;
  }

  apiFetch('/api/v1/config/storage/update', {
    method: 'PUT',
    body: JSON.stringify({
      type: type,
      database_url: url,
      file_path: filePath,
      ssl_enabled: sslEnabled
    })
  }).then(data => {
    if (data.success) {
      toast('Configuração salva! Reinicie o PicoClaw para aplicar.');
      const resultDiv = document.getElementById('storage-test-result');
      resultDiv.className = 'storage-result success show';
      resultDiv.innerHTML = '<strong>✓ Configuração salva com sucesso!</strong><br>Por favor, reinicie o PicoClaw para que as alterações entrem em efeito.';
    } else {
      toast('Erro ao salvar configuração', true);
    }
  }).catch(err => {
    toast('Erro ao salvar: ' + err.message, true);
  });
}

// Listen to storage type changes
document.addEventListener('DOMContentLoaded', () => {
  const storageTypeSelect = document.getElementById('storage-type');
  if (storageTypeSelect) {
    storageTypeSelect.addEventListener('change', toggleStorageFields);
  }
});

// ─── QR Code ─────────────────────────────────────────────────────────
function handleQREvent(qr) {
  qrState = qr;

  const container = document.getElementById('qr-image');
  const status = document.getElementById('qr-status');
  const instructions = document.getElementById('qr-instructions');
  const actions = document.getElementById('qr-actions');
  const btn = document.getElementById('btn-request-qr');

  if (!container) return;

  // Stop polling if active
  if (qrPollTimer) { clearTimeout(qrPollTimer); qrPollTimer = null; }

  switch (qr.event) {
    case 'code':
      container.innerHTML = qr.svg || '';
      status.innerHTML = '<div class="qr-scan-prompt">Escaneie o QR Code</div>';
      instructions.style.display = 'block';
      if (actions) actions.style.display = 'none';
      updateSidebarQR('pending');
      showView('qr');
      break;

    case 'success':
      container.innerHTML = '';
      status.innerHTML = '<div class="qr-success">WhatsApp conectado com sucesso!</div>';
      instructions.style.display = 'none';
      if (actions) actions.style.display = 'none';
      qrState = null;
      updateSidebarQR('connected');
      toast('WhatsApp conectado');
      setTimeout(() => {
        loadChannels();
        loadOverview();
      }, 1000);
      break;

    case 'timeout':
      container.innerHTML = '';
      status.innerHTML = '<div class="qr-error">QR code expirado. Clique abaixo para tentar novamente.</div>';
      instructions.style.display = 'none';
      if (actions) actions.style.display = 'block';
      if (btn) { btn.disabled = false; btn.textContent = 'Tentar Novamente'; }
      qrState = null;
      updateSidebarQR('error');
      toast('QR code expirou', true);
      break;

    case 'error':
      container.innerHTML = '';
      status.innerHTML = '<div class="qr-error">Erro na autenticacao. Clique abaixo para tentar novamente.</div>';
      instructions.style.display = 'none';
      if (actions) actions.style.display = 'block';
      if (btn) { btn.disabled = false; btn.textContent = 'Tentar Novamente'; }
      qrState = null;
      updateSidebarQR('error');
      toast('Erro na autenticacao WhatsApp', true);
      break;
  }
}

function updateSidebarQR(state) {
  const dot = document.getElementById('sidebar-qr-dot');
  const label = document.getElementById('sidebar-qr-status');
  if (!dot || !label) return;

  dot.className = 'channel-dot';
  switch (state) {
    case 'connected':
      dot.classList.add('running');
      label.textContent = 'conectado';
      break;
    case 'pending':
      label.textContent = 'QR pendente';
      break;
    case 'error':
      label.textContent = 'erro';
      break;
    default:
      label.textContent = 'desconectado';
  }
}

function checkPendingQR() {
  // Check if there's a pending QR code
  apiFetch('/api/v1/whatsapp/qr').then(data => {
    if (data && data.event === 'code' && data.svg) {
      handleQREvent(data);
      return;
    }

    // No pending QR - check channel status to see if already connected
    apiFetch('/api/v1/channels').then(channels => {
      const wa = channels && channels['whatsapp'];
      if (wa && wa.running) {
        // Already connected
        const status = document.getElementById('qr-status');
        const actions = document.getElementById('qr-actions');
        if (status) status.innerHTML = '<div class="qr-success">WhatsApp conectado!</div>';
        if (actions) actions.style.display = 'none';
        updateSidebarQR('connected');
      } else {
        // Not connected, show button
        const actions = document.getElementById('qr-actions');
        if (actions) actions.style.display = 'block';
        updateSidebarQR(wa ? 'disconnected' : 'disconnected');
      }
    }).catch(() => {});
  }).catch(() => {});
}

function requestWhatsAppQR() {
  const btn = document.getElementById('btn-request-qr');
  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Conectando...';
  }

  const status = document.getElementById('qr-status');
  if (status) {
    status.innerHTML = '<div class="qr-waiting">Iniciando WhatsApp... aguarde o QR code</div>';
  }

  apiFetch('/api/v1/whatsapp/connect', { method: 'POST' }).then(data => {
    if (data.success) {
      // Start polling for QR code
      pollForQR(0);
    } else {
      if (btn) {
        btn.disabled = false;
        btn.textContent = 'Conectar WhatsApp';
      }
      if (data.error && data.error.includes('already connected')) {
        if (status) status.innerHTML = '<div class="qr-success">WhatsApp ja esta conectado!</div>';
        updateSidebarQR('connected');
        if (btn) btn.style.display = 'none';
      } else {
        toast('Erro: ' + (data.error || 'Falha ao conectar'), true);
        if (status) status.innerHTML = '<div class="qr-error">Erro ao conectar. Tente novamente.</div>';
      }
    }
  }).catch(err => {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Conectar WhatsApp';
    }
    toast('Erro ao solicitar conexao', true);
  });
}

let qrPollTimer = null;
function pollForQR(attempt) {
  if (attempt > 30) {
    const btn = document.getElementById('btn-request-qr');
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Tentar Novamente';
    }
    const status = document.getElementById('qr-status');
    if (status) status.innerHTML = '<div class="qr-error">Tempo esgotado. Tente novamente.</div>';
    return;
  }

  if (qrPollTimer) clearTimeout(qrPollTimer);

  qrPollTimer = setTimeout(() => {
    apiFetch('/api/v1/whatsapp/qr').then(data => {
      if (data && data.event === 'code' && data.svg) {
        handleQREvent(data);
      } else {
        pollForQR(attempt + 1);
      }
    }).catch(() => {
      pollForQR(attempt + 1);
    });
  }, 1000);
}

// ─── Setup Status ────────────────────────────────────────────────────
function checkSetupStatus() {
  apiFetch('/api/v1/status').then(data => {
    const container = document.getElementById('setup-alerts');
    if (!container) return;

    const setup = data.setup;
    if (!setup || !setup.issues || setup.issues.length === 0) {
      container.style.display = 'none';
      return;
    }

    const icons = {
      error: '⚠',
      warning: '💡'
    };

    const actionLabels = {
      settings: 'Configurar'
    };

    container.innerHTML = setup.issues.map(issue => {
      const icon = icons[issue.type] || '📋';
      const actionBtn = issue.action
        ? `<div class="setup-alert-action"><button class="btn-alert-action" onclick="showView('${issue.action}')">${actionLabels[issue.action] || 'Abrir'}</button></div>`
        : '';

      return `<div class="setup-alert ${issue.type}">
        <div class="setup-alert-icon">${icon}</div>
        <div class="setup-alert-content">
          <div class="setup-alert-title">${escapeHtml(issue.title)}</div>
          <div class="setup-alert-message">${escapeHtml(issue.message)}</div>
        </div>
        ${actionBtn}
      </div>`;
    }).join('');

    container.style.display = 'flex';
  }).catch(() => {});
}

// Handle Enter key on login
document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && document.getElementById('login-screen').style.display !== 'none') {
    doLogin();
  }
});
