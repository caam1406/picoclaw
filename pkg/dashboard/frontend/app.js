// PicoClaw Dashboard - Vanilla JS

let token = localStorage.getItem('picoclaw_token') || '';
let ws = null;
let currentContact = null; // {channel, id} of contact being edited
let liveMessages = [];
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
      return;
    }

    list.innerHTML = Object.entries(data).map(([name, info]) => {
      const dotClass = info.running ? 'running' : '';
      return `<div class="sidebar-item">
        <span class="channel-dot ${dotClass}"></span>
        <span>${capitalize(name)}</span>
        <span style="margin-left:auto;font-size:11px;color:var(--text-muted)">${info.running ? 'ativo' : 'parado'}</span>
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
      return `<div class="sidebar-item" onclick="editContact('${escapeAttr(c.channel)}','${escapeAttr(c.contact_id)}')">
        <span>${escapeHtml(label)}</span>
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
}

// ─── Contact CRUD ───────────────────────────────────────────────────
function editContact(channel, id) {
  currentContact = { channel, id };

  apiFetch('/api/v1/contacts/' + channel + '/' + id).then(data => {
    document.getElementById('contact-channel').value = data.channel;
    document.getElementById('contact-id').value = data.contact_id;
    document.getElementById('contact-name').value = data.display_name || '';
    document.getElementById('contact-instructions').value = data.instructions || '';
    document.getElementById('contact-title').textContent = data.display_name || data.contact_id;
    document.getElementById('contact-subtitle').textContent = data.channel + ' / ' + data.contact_id;
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

function showAddContact() {
  document.getElementById('modal-add-contact').classList.add('active');
  document.getElementById('new-contact-channel').value = 'whatsapp';
  document.getElementById('new-contact-id').value = '';
  document.getElementById('new-contact-name').value = '';
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

  apiFetch('/api/v1/contacts/' + channel + '/' + id, {
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
    document.getElementById('contact-title').textContent = name || id;
    document.getElementById('contact-subtitle').textContent = channel + ' / ' + id;
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

  apiFetch('/api/v1/contacts/' + currentContact.channel + '/' + currentContact.id, {
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

  apiFetch('/api/v1/contacts/' + currentContact.channel + '/' + currentContact.id, {
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

// ─── Toast ──────────────────────────────────────────────────────────
function toast(msg, isError) {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = 'toast' + (isError ? ' error' : '');
  el.classList.add('show');
  setTimeout(() => el.classList.remove('show'), 2500);
}

// ─── Helpers ────────────────────────────────────────────────────────
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

// Handle Enter key on login
document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && document.getElementById('login-screen').style.display !== 'none') {
    doLogin();
  }
});
