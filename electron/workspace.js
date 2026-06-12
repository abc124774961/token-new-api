const api = window.electronBrowser;

const state = {
  accounts: [],
  tabs: [],
  activeTabId: '',
  maxTabs: 16,
  loadingAccounts: false
};

const els = {
  accountList: document.getElementById('accountList'),
  accountSummary: document.getElementById('accountSummary'),
  accountSearch: document.getElementById('accountSearch'),
  statusFilter: document.getElementById('statusFilter'),
  refreshAccounts: document.getElementById('refreshAccounts'),
  openAdmin: document.getElementById('openAdmin'),
  browserStage: document.getElementById('browserStage'),
  emptyState: document.getElementById('emptyState'),
  tabDock: document.getElementById('tabDock'),
  urlInput: document.getElementById('urlInput'),
  navigateTab: document.getElementById('navigateTab'),
  reloadTab: document.getElementById('reloadTab'),
  goBack: document.getElementById('goBack'),
  goForward: document.getElementById('goForward'),
  activeProfile: document.getElementById('activeProfile'),
  toast: document.getElementById('toast')
};

function accountLabel(account) {
  return (
    account.credential_label ||
    account.credential_uid ||
    account.account_identity?.account_id ||
    account.credential_ref?.credential_subject_fingerprint ||
    `#${Number(account.credential_index) + 1}`
  );
}

function accountSubtitle(account) {
  return [account.channel_name || `Channel #${account.channel_id}`, account.account_identity?.account_type, account.proxy?.masked_address]
    .filter(Boolean)
    .join(' · ');
}

function showToast(message) {
  els.toast.textContent = message;
  els.toast.classList.add('show');
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => els.toast.classList.remove('show'), 3200);
}

async function loadAccounts() {
  state.loadingAccounts = true;
  renderAccounts();
  try {
    const keyword = els.accountSearch.value.trim();
    const status = els.statusFilter.value;
    const data = await api.listAccounts({ keyword, status });
    state.accounts = data.items || [];
    els.accountSummary.textContent = `${state.accounts.length} 个账号 · 最多同时打开 ${state.maxTabs} 个浏览器`;
  } catch (err) {
    state.accounts = [];
    els.accountSummary.textContent = '账号加载失败';
    showToast(err.message || '账号加载失败');
  } finally {
    state.loadingAccounts = false;
    renderAccounts();
  }
}

function renderAccounts() {
  if (state.loadingAccounts) {
    els.accountList.innerHTML = '<div class="account-card"><div><div class="account-title">账号加载中</div><div class="account-meta"><span class="chip muted">请稍候</span></div></div></div>';
    return;
  }
  if (!state.accounts.length) {
    els.accountList.innerHTML = '<div class="account-card"><div><div class="account-title">暂无账号</div><div class="account-meta"><span class="chip muted">请先在管理后台导入渠道账号</span></div></div></div>';
    return;
  }
  els.accountList.innerHTML = '';
  for (const account of state.accounts) {
    const card = document.createElement('article');
    card.className = 'account-card';
    card.addEventListener('dblclick', () => openAccount(account));
    const statusClass = account.key_enabled ? 'chip' : 'chip warn';
    card.innerHTML = `
      <div>
        <div class="account-title" title="${escapeHTML(accountLabel(account))}">${escapeHTML(accountLabel(account))}</div>
        <div class="account-meta">
          <span class="${statusClass}">${account.key_enabled ? '可用' : '禁用'}</span>
          <span class="chip muted" title="${escapeHTML(accountSubtitle(account))}">${escapeHTML(accountSubtitle(account) || '未绑定代理')}</span>
          <span class="chip muted">#${Number(account.credential_index) + 1}</span>
        </div>
      </div>
      <button class="open-account">浏览器中打开</button>
    `;
    card.querySelector('button').addEventListener('click', () => openAccount(account));
    els.accountList.appendChild(card);
  }
}

async function openAccount(account) {
  try {
    await api.openAccount({ profileKey: account.profile_key });
  } catch (err) {
    showToast(err.message || '打开账号失败');
  }
}

function renderTabs(snapshot) {
  state.tabs = snapshot.tabs || [];
  state.activeTabId = snapshot.activeTabId || '';
  state.maxTabs = snapshot.maxTabs || state.maxTabs;
  const active = state.tabs.find((tab) => tab.id === state.activeTabId);
  els.emptyState.style.display = active ? 'none' : 'grid';
  els.urlInput.value = active?.url || '';
  els.activeProfile.textContent = active
    ? `${accountLabel(active.account)} · ${fingerprintLabel(active.fingerprintStatus)} · ${active.profile?.timezone || '--'} · ${profileScreenLabel(active.profile)} · ${active.account?.proxy?.masked_address || '直连'}`
    : '未打开账号';
  els.activeProfile.title = active ? profileTitle(active.profile) : '';

  els.tabDock.innerHTML = '';
  for (const tab of state.tabs) {
    const card = document.createElement('div');
    card.className = `tab-card${tab.active ? ' active' : ''}`;
    card.innerHTML = `
      ${
        tab.thumbnail
          ? `<img class="tab-thumb" src="${tab.thumbnail}" alt="thumbnail" />`
          : '<div class="tab-thumb placeholder">等待截图</div>'
      }
      <button class="tab-close" title="关闭">×</button>
      <div class="tab-info">
        <div class="tab-title" title="${escapeHTML(tab.title || accountLabel(tab.account))}">${escapeHTML(tab.title || accountLabel(tab.account))}</div>
        <div class="tab-status ${fingerprintStatusClass(tab.fingerprintStatus)}">${escapeHTML(fingerprintLabel(tab.fingerprintStatus))}</div>
        <div class="tab-url" title="${escapeHTML(tab.url || '')}">${escapeHTML(tab.url || '')}</div>
      </div>
    `;
    card.addEventListener('click', () => api.activateTab(tab.id).catch((err) => showToast(err.message)));
    card.querySelector('.tab-close').addEventListener('click', (event) => {
      event.stopPropagation();
      api.closeTab(tab.id).catch((err) => showToast(err.message));
    });
    els.tabDock.appendChild(card);
  }
  syncBrowserBounds();
}

function fingerprintLabel(status) {
  if (status?.applying) return '指纹应用中';
  if (status?.applied) return '指纹已应用';
  if (Array.isArray(status?.warnings) && status.warnings.length) return '指纹部分失败';
  return '指纹待应用';
}

function fingerprintStatusClass(status) {
  if (status?.applying) return 'pending';
  if (status?.applied) return 'ok';
  if (Array.isArray(status?.warnings) && status.warnings.length) return 'warn';
  return 'pending';
}

function profileScreenLabel(profile) {
  if (!profile?.screen) {
    return profile?.browserPlatform || '--';
  }
  return `${profile.browserPlatform || '--'} ${profile.screen.width}x${profile.screen.height}`;
}

function profileTitle(profile) {
  if (!profile) return '';
  return [
    `UA: ${profile.userAgent || ''}`,
    `Language: ${profile.language || ''}`,
    `Timezone: ${profile.timezone || ''}`,
    `Screen: ${profile.screen?.width || '--'}x${profile.screen?.height || '--'} @${profile.screen?.pixelRatio || 1}`,
    `CPU/Memory: ${profile.hardwareConcurrency || '--'} cores / ${profile.deviceMemory || '--'} GB`,
    `WebGL: ${profile.webgl?.vendor || '--'} / ${profile.webgl?.renderer || '--'}`
  ].join('\n');
}

function syncBrowserBounds() {
  const rect = els.browserStage.getBoundingClientRect();
  api.setLayout({
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height
  }).catch(() => {});
}

function escapeHTML(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

els.refreshAccounts.addEventListener('click', loadAccounts);
els.openAdmin.addEventListener('click', () => api.openAdmin().catch((err) => showToast(err.message)));
els.accountSearch.addEventListener('keydown', (event) => {
  if (event.key === 'Enter') loadAccounts();
});
els.statusFilter.addEventListener('change', loadAccounts);
els.navigateTab.addEventListener('click', () => {
  api.navigate({ tabId: state.activeTabId, url: els.urlInput.value }).catch((err) => showToast(err.message));
});
els.urlInput.addEventListener('keydown', (event) => {
  if (event.key === 'Enter') {
    api.navigate({ tabId: state.activeTabId, url: els.urlInput.value }).catch((err) => showToast(err.message));
  }
});
els.reloadTab.addEventListener('click', () => api.reload(state.activeTabId).catch((err) => showToast(err.message)));
els.goBack.addEventListener('click', () => api.back(state.activeTabId).catch((err) => showToast(err.message)));
els.goForward.addEventListener('click', () => api.forward(state.activeTabId).catch((err) => showToast(err.message)));
window.addEventListener('resize', () => requestAnimationFrame(syncBrowserBounds));

api.onTabsUpdated(renderTabs);
api.getTabs().then(renderTabs).catch(() => {});
loadAccounts();
