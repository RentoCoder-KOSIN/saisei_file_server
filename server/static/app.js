'use strict';

let currentRole = 'student';
let currentUser = '';
let allFiles = []; // 検索・フィルター用キャッシュ

const TASK_API = '/api'; // Goのリバースプロキシ経由（環境に依存しない）

// ─── Login ───
async function login() {
    const username = document.getElementById('login-username').value.trim();
    const password = document.getElementById('login-password').value;

    if (!username || !password) {
        showToast('❌ IDとパスワードを入力してください', 'error');
        return;
    }

    const res = await fetch('/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
    });

    if (!res.ok) {
        showToast('❌ IDまたはパスワードが違います', 'error');
        return;
    }

    const data = await res.json();
    currentUser = data.username;
    currentRole = data.role;

    const badge = document.getElementById('user-badge');
    if (currentRole === 'teacher') {
        badge.textContent = '👨‍🏫 ' + currentUser;
        badge.className = 'user-badge teacher';
        document.body.classList.add('is-teacher');
    } else {
        badge.textContent = '👨‍🎓 ' + currentUser;
        badge.className = 'user-badge';
        document.body.classList.remove('is-teacher');
    }

    document.getElementById('login-screen').classList.add('hidden');
    showToast('✅ ログインしました', 'success');
    fetchFiles();
    fetchTasks();
    if (currentRole === 'teacher') fetchLateNotifications();
    if (currentRole === 'teacher') fetchUsers();
}

// ─── Logout ───
function logout() {
    document.getElementById('login-screen').classList.remove('hidden');
    document.getElementById('login-username').value = '';
    document.getElementById('login-password').value = '';
    document.body.classList.remove('is-teacher');
}

// ─── Drag & Drop ───
function onDragOver(e) {
    e.preventDefault();
    e.currentTarget.classList.add('dragover');
}
function onDragLeave(e) {
    e.currentTarget.classList.remove('dragover');
}
function onDrop(e) {
    e.preventDefault();
    e.currentTarget.classList.remove('dragover');
    const files = e.dataTransfer.files;
    if (files.length > 0) uploadFolder(files);
}

// ─── Add User ───
async function addUser() {
    const username = document.getElementById('new-username').value.trim();
    const password = document.getElementById('new-password').value;
    const role     = document.getElementById('new-role').value;

    if (!username || !password) {
        showToast('❌ ユーザー名とパスワードを入力してください', 'error');
        return;
    }
    const res = await fetch('/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        // operator: 操作者（ログイン中の先生）を追加
        body: JSON.stringify({ operator: currentUser, username, password, role }),
    });
    const text = await res.text();
    if (res.ok) {
        showToast(text, 'success');
        document.getElementById('new-username').value = '';
        document.getElementById('new-password').value = '';
        fetchUsers();
    } else {
        showToast('❌ ' + text, 'error');
    }
}

// ─── Delete User ───
async function deleteUser(username) {
    // operator: 操作者（ログイン中の先生）を追加
    const res = await fetch(
        `/users?username=${encodeURIComponent(username)}&operator=${encodeURIComponent(currentUser)}`,
        { method: 'DELETE' }
    );
    const text = await res.text();
    if (res.ok) {
        showToast(text, 'success');
        fetchUsers();
    } else {
        showToast('❌ ' + text, 'error');
    }
}

// ─── Fetch Users (先生のみ) ───
async function fetchUsers() {
    try {
        // operator: 操作者（ログイン中の先生）を追加
        const res = await fetch(`/users?operator=${encodeURIComponent(currentUser)}`);
        if (!res.ok) return;
        const users = await res.json();
        renderUsers(users);
    } catch (e) {
        console.warn('ユーザー一覧取得失敗', e);
    }
}

function renderUsers(users) {
    const list = document.getElementById('user-list');
    if (!list) return;

    if (!users || users.length === 0) {
        list.innerHTML = `<p class="notif-empty">// ユーザーがいません</p>`;
        return;
    }

    list.innerHTML = users.map(u => `
        <div class="user-list-item">
            <span class="user-list-name">
                ${u.role === 'teacher' ? '👨‍🏫' : '👨‍🎓'} ${u.username}
            </span>
            <span class="user-list-role ${u.role === 'teacher' ? 'teacher' : ''}">${u.role}</span>
            ${u.username !== 'admin' ? `
            <button class="action-btn del" style="padding:4px 10px; font-size:10px;"
                onclick="deleteUser('${u.username}')">✕ 削除</button>
            ` : ''}
        </div>
    `).join('');
}

// ─── Change Password ───
async function changePassword() {
    const oldPassword = document.getElementById('old-password').value;
    const newPassword = document.getElementById('new-passwd').value;

    if (!oldPassword || !newPassword) {
        showToast('❌ パスワードを入力してください', 'error');
        return;
    }
    const res = await fetch('/users/passwd', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            username:     currentUser,
            old_password: oldPassword,
            new_password: newPassword,
        }),
    });
    const text = await res.text();
    if (res.ok) {
        showToast(text, 'success');
        document.getElementById('old-password').value = '';
        document.getElementById('new-passwd').value = '';
    } else {
        showToast('❌ ' + text, 'error');
    }
}

// ─── Upload Folder(s) ───
// files: FileList — 複数フォルダをまとめてドロップされた場合も対応
async function uploadFolder(files) {
    const taskId = document.getElementById('upload-task-select').value;

    // フォルダ名でグループ分け
    const groups = {};
    for (const file of files) {
        const folder = (file.webkitRelativePath || file.name).split('/')[0];
        if (!groups[folder]) groups[folder] = [];
        groups[folder].push(file);
    }

    const folderNames = Object.keys(groups);
    if (folderNames.length === 0) return;

    // 各フォルダのプログレスバーをまとめて描画
    const container = document.getElementById('multi-progress-container');
    if (!container) {
        // index.html が古い場合のフォールバック
        for (const folder of folderNames) {
            await uploadSingleFolderLegacy(folder, groups[folder], taskId);
        }
        setTimeout(() => { fetchFiles(); fetchTasks(); if (currentRole === 'teacher') fetchLateNotifications(); }, 400);
        return;
    }
    container.innerHTML = '';
    container.classList.add('show');

    const bars = {};
    for (const folder of folderNames) {
        const wrap = document.createElement('div');
        wrap.className = 'multi-progress-item';
        wrap.innerHTML = `
            <div class="multi-progress-label" id="label-${CSS.escape(folder)}">${folder} — 待機中...</div>
            <div class="progress-track">
                <div class="progress-fill" id="fill-${CSS.escape(folder)}" style="width:0%"></div>
            </div>
        `;
        container.appendChild(wrap);
        bars[folder] = {
            fill:  document.getElementById(`fill-${CSS.escape(folder)}`),
            label: document.getElementById(`label-${CSS.escape(folder)}`),
        };
    }

    // 各フォルダを並列アップロード
    await Promise.all(folderNames.map(folder =>
        uploadSingleFolder(folder, groups[folder], taskId, bars[folder])
    ));

    setTimeout(() => {
        container.classList.remove('show');
        container.innerHTML = '';
        fetchFiles();
        fetchTasks();
        if (currentRole === 'teacher') fetchLateNotifications();
    }, 600);
}

// index.html が古い環境向けフォールバック（プログレスバーなし）
async function uploadSingleFolderLegacy(folderName, files, taskId) {
    for (const file of files) {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('path', file.webkitRelativePath || file.name);
        await fetch('/upload', { method: 'POST', body: formData });
    }
    let url = `/finalize?folder=${encodeURIComponent(folderName)}&username=${encodeURIComponent(currentUser)}`;
    if (taskId) url += `&task_id=${encodeURIComponent(taskId)}`;
    await fetch(url, { method: 'POST' });
    showToast(`✅ ${folderName}.zip をアップロードしました`, 'success');
}

// 1フォルダ分のアップロード処理
async function uploadSingleFolder(folderName, files, taskId, bar) {
    let done = 0;
    const total = files.length;
    bar.label.textContent = `${folderName} — 0 / ${total}`;

    for (const file of files) {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('path', file.webkitRelativePath || file.name);
        await fetch('/upload', { method: 'POST', body: formData });
        done++;
        const pct = Math.floor((done / total) * 100);
        bar.fill.style.width = pct + '%';
        bar.label.textContent = `${folderName} — ${done} / ${total} (${pct}%)`;
    }

    let url = `/finalize?folder=${encodeURIComponent(folderName)}&username=${encodeURIComponent(currentUser)}`;
    if (taskId) url += `&task_id=${encodeURIComponent(taskId)}`;
    await fetch(url, { method: 'POST' });

    bar.fill.style.width = '100%';
    bar.label.textContent = `${folderName} ✅`;
    showToast(`✅ ${folderName}.zip をアップロードしました`, 'success');
}

// ─── Fetch Files ───
async function fetchFiles() {
    try {
        const res = await fetch('/files');
        allFiles = await res.json() || [];
        populateUserFilter(allFiles);
        filterFiles();
    } catch (e) {
        showToast('❌ ファイル一覧の取得に失敗しました', 'error');
    }
}

// ─── Filter Files ───
function filterFiles() {
    const q    = (document.getElementById('search-input')?.value || '').toLowerCase();
    const date = document.getElementById('search-date')?.value || '';
    const user = document.getElementById('search-user')?.value || '';

    const filtered = allFiles.filter(f => {
        if (q && !f.filename.toLowerCase().includes(q) && !f.username.toLowerCase().includes(q)) return false;
        if (date && !f.uploaded_at.startsWith(date)) return false;
        if (user && f.username !== user) return false;
        return true;
    });
    renderFiles(filtered);
}

// ユーザーフィルターのプルダウンを生成
function populateUserFilter(files) {
    const select = document.getElementById('search-user');
    if (!select) return;
    const users = [...new Set(files.map(f => f.username))].sort();
    const current = select.value;
    select.innerHTML = `<option value="">── ユーザーで絞り込み ──</option>` +
        users.map(u => `<option value="${u}" ${u === current ? 'selected' : ''}>${u}</option>`).join('');
}

// ─── Render Files ───
function renderFiles(files) {
    const list = document.getElementById('file-list');
    document.getElementById('file-count').textContent = files ? files.length : 0;

    if (!files || files.length === 0) {
        list.innerHTML = `<div class="empty-state"><div class="empty-icon">🗂️</div><p>// まだファイルがありません</p></div>`;
        return;
    }

    const isTeacher = document.body.classList.contains('is-teacher');
    list.innerHTML = files.map((f, i) => {
        const parts = f.filename.split('/');
        // 新構成: {username}/{task_id}/{folder}.zip → フォルダ名だけ表示
        // 旧構成: {folder}.zip → そのまま表示
        const basename = parts.pop();
        const taskLabel = parts.length >= 2
            ? `<span>📋 課題ID: ${parts[1]}</span>`
            : '';
        return `
        <div class="file-item" style="animation-delay:${i * 0.05}s">
            <div class="file-ext zip">${getExt(f.filename)}</div>
            <div class="file-info">
                <div class="file-name">${basename}</div>
                <div class="file-meta">
                    <span>📤 ${f.username}</span>
                    <span>📅 ${f.uploaded_at}</span>
                    ${taskLabel}
                </div>
            </div>
            <div class="file-actions">
                <button class="action-btn dl" onclick="downloadFile('${f.filename}')">↓ DL</button>
                ${isTeacher ? `
                <button class="action-btn run" onclick="runFile('${f.filename}')">▶ 実行</button>
                <button class="action-btn del" onclick="deleteFile('${f.filename}')">✕ 削除</button>
                ` : ''}
            </div>
        </div>`;
    }).join('');
}

// ─── Download ───
function downloadFile(name) {
    const a = document.createElement('a');
    a.href = '/download?name=' + encodeURIComponent(name);
    a.download = name;
    a.click();
}

// ─── Run（operator追加）───
async function runFile(name) {
    showToast('▶ ' + name + ' を実行中...', 'success');
    const res = await fetch(
        `/run?name=${encodeURIComponent(name)}&operator=${encodeURIComponent(currentUser)}`,
        { method: 'POST' }
    );
    const data = await res.json();
    if (!res.ok) { showToast('❌ 実行に失敗しました', 'error'); return; }
    showResult(name, data.stdout, data.stderr);
}

function showResult(name, stdout, stderr) {
    const existing = document.getElementById('result-panel');
    if (existing) existing.remove();
    const panel = document.createElement('div');
    panel.id = 'result-panel';
    panel.innerHTML = `
        <div class="result-header">
            <span>▶ 実行結果：${name}</span>
            <button onclick="document.getElementById('result-panel').remove()">✕</button>
        </div>
        <pre class="result-stdout">${stdout || '// 出力なし'}</pre>
        ${stderr ? `<pre class="result-stderr">${stderr}</pre>` : ''}
    `;
    document.querySelector('main').appendChild(panel);
}

// ─── Delete File（operator追加）───
async function deleteFile(name) {
    const res = await fetch(
        `/delete?name=${encodeURIComponent(name)}&operator=${encodeURIComponent(currentUser)}`,
        { method: 'DELETE' }
    );
    if (res.ok) {
        showToast('🗑️ ' + name + ' を削除しました', 'error');
        fetchFiles();
    } else {
        showToast('❌ 削除に失敗しました', 'error');
    }
}

// ════════════════════════════════════════
//  課題管理
// ════════════════════════════════════════

let allTasks = [];
let mySubmittedTaskIds = new Set();

async function fetchTasks() {
    try {
        const res = await fetch(TASK_API + '/tasks');
        if (!res.ok) return;
        allTasks = await res.json();

        if (currentRole === 'student') {
            try {
                const sRes = await fetch(`${TASK_API}/submissions/mine?username=${encodeURIComponent(currentUser)}`);
                if (sRes.ok) {
                    const ids = await sRes.json();
                    mySubmittedTaskIds = new Set(ids);
                }
            } catch (e) { /* タスクサーバー未接続時は無視 */ }
        }

        renderTasks(allTasks);
        renderTaskSelect(allTasks);
    } catch (e) {
        console.warn('タスクサーバーに接続できません', e);
    }
}

function renderTaskSelect(tasks) {
    const select = document.getElementById('upload-task-select');
    if (!select) return;

    const filtered = tasks.filter(t => {
        if (t.is_expired) return false;
        if (currentRole === 'student' && mySubmittedTaskIds.has(t.id)) return false;
        return true;
    });

    select.innerHTML = `<option value="">── 課題を選択（任意）──</option>` +
        filtered.map(t =>
            `<option value="${t.id}">${t.title}（期限：${formatDeadline(t.deadline)}）</option>`
        ).join('');
}

function renderTasks(tasks) {
    const list  = document.getElementById('task-list');
    const count = document.getElementById('task-count');
    if (!list) return;

    const isTeacher = document.body.classList.contains('is-teacher');
    const visible = isTeacher
        ? tasks
        : tasks.filter(t => !mySubmittedTaskIds.has(t.id));

    count.textContent = visible.length;

    if (!visible || visible.length === 0) {
        list.innerHTML = `<div class="empty-state"><div class="empty-icon">✅</div><p>// すべての課題を提出済みです</p></div>`;
        return;
    }

    list.innerHTML = visible.map((t, i) => {
        const badge = t.is_expired
            ? `<span class="task-badge expired">期限切れ</span>`
            : `<span class="task-badge active">受付中</span>`;
        return `
        <div class="task-item${t.is_expired ? ' expired' : ''}" style="animation-delay:${i * 0.05}s">
            <div class="task-main">
                <div class="task-title">${t.title} ${badge}</div>
                <div class="task-desc">${t.description}</div>
                <div class="file-meta" style="margin-top:6px;">
                    <span>⏰ 期限：${formatDeadline(t.deadline)}</span>
                    <span>👨‍🏫 ${t.created_by}</span>
                </div>
            </div>
            <div class="file-actions">
                ${isTeacher ? `
                <button class="action-btn run" onclick="showSubmissionStatus(${t.id}, '${escHtml(t.title)}')">📊 提出状況</button>
                <button class="action-btn del" onclick="deleteTask(${t.id})">✕ 削除</button>
                ` : ''}
            </div>
        </div>`;
    }).join('');
}

function formatDeadline(iso) {
    const d = new Date(iso);
    return d.toLocaleString('ja-JP', { year:'numeric', month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit' });
}

async function createTask() {
    const title    = document.getElementById('task-title').value.trim();
    const desc     = document.getElementById('task-desc').value.trim();
    const deadline = document.getElementById('task-deadline').value;

    if (!title || !desc || !deadline) {
        showToast('❌ すべての項目を入力してください', 'error');
        return;
    }
    const res = await fetch(TASK_API + '/tasks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title, description: desc, deadline, created_by: currentUser }),
    });
    const data = await res.json();
    if (res.ok) {
        showToast(data.message, 'success');
        document.getElementById('task-title').value = '';
        document.getElementById('task-desc').value = '';
        document.getElementById('task-deadline').value = '';
        fetchTasks();
    } else {
        showToast('❌ ' + (data.detail || '作成失敗'), 'error');
    }
}

async function deleteTask(taskId) {
    const res = await fetch(`${TASK_API}/tasks/${taskId}?username=${encodeURIComponent(currentUser)}`, {
        method: 'DELETE',
    });
    const data = await res.json();
    if (res.ok) {
        showToast(data.message, 'success');
        fetchTasks();
    } else {
        showToast('❌ ' + (data.detail || '削除失敗'), 'error');
    }
}

async function showSubmissionStatus(taskId, taskTitle) {
    const res = await fetch(`${TASK_API}/submissions/status/${taskId}`);
    if (!res.ok) { showToast('❌ 提出状況の取得に失敗しました', 'error'); return; }
    const data = await res.json();

    const existing = document.getElementById('status-modal');
    if (existing) existing.remove();

    const submitted = data.students.filter(s => s.status === 'submitted').length;
    const late      = data.students.filter(s => s.status === 'late').length;
    const pending   = data.students.filter(s => s.status === 'pending').length;

    const modal = document.createElement('div');
    modal.id = 'status-modal';
    modal.innerHTML = `
        <div class="modal-overlay" onclick="document.getElementById('status-modal').remove()"></div>
        <div class="modal-card">
            <div class="modal-header">
                <span>📊 提出状況：${taskTitle}</span>
                <button onclick="document.getElementById('status-modal').remove()">✕</button>
            </div>
            <div class="modal-summary">
                <div class="summary-chip submitted">✅ 提出済み ${submitted}</div>
                <div class="summary-chip late">⚠️ 遅延 ${late}</div>
                <div class="summary-chip pending">⏳ 未提出 ${pending}</div>
            </div>
            <div class="modal-body">
                ${data.students.map(s => `
                <div class="status-row ${s.status}">
                    <span class="status-name">👤 ${s.username}</span>
                    <span class="status-badge ${s.status}">${statusLabel(s.status)}</span>
                    <span class="status-time">${s.submitted_at ? s.submitted_at.slice(0,16).replace('T',' ') : '──'}</span>
                </div>`).join('')}
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function statusLabel(s) {
    if (s === 'submitted') return '✅ 提出済み';
    if (s === 'late')      return '⚠️ 遅延提出';
    return '⏳ 未提出';
}

async function fetchLateNotifications() {
    try {
        const res = await fetch(TASK_API + '/notifications/late');
        if (!res.ok) return;
        const data = await res.json();
        renderLateNotifications(data);
    } catch (e) {
        console.warn('遅延通知の取得に失敗', e);
    }
}

function renderLateNotifications(items) {
    const panel = document.getElementById('late-notifications');
    if (!panel) return;
    if (!items || items.length === 0) {
        panel.innerHTML = `<p class="notif-empty">// 遅延提出なし</p>`;
        return;
    }
    panel.innerHTML = items.map(n => `
        <div class="notif-item">
            <span>⚠️ <strong>${n.username}</strong> が「${n.task_title}」を遅延提出</span>
            <span class="notif-time">${n.submitted_at.slice(0,16).replace('T',' ')}</span>
        </div>
    `).join('');
}

// ─── Toast ───
function showToast(msg, type = 'success') {
    const t = document.getElementById('toast');
    t.textContent = msg;
    t.className = 'toast ' + type + ' show';
    setTimeout(() => t.classList.remove('show'), 3000);
}

// ─── Utils ───
function getExt(filename) {
    const parts = filename.split('.');
    return parts.length > 1 ? parts[parts.length - 1] : 'file';
}
function escHtml(str) {
    return str.replace(/'/g, "\\'").replace(/"/g, '&quot;');
}