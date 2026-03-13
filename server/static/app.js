'use strict';

let currentRole = 'student';
let currentUser = '';

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
        body: JSON.stringify({ username, password, role }),
    });

    const text = await res.text();
    if (res.ok) {
        showToast(text, 'success');
        document.getElementById('new-username').value = '';
        document.getElementById('new-password').value = '';
    } else {
        showToast('❌ ' + text, 'error');
    }
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

// ─── Upload Folder ───
async function uploadFolder(files) {
    const progress = document.getElementById('progress-bar');
    const fill     = document.getElementById('progress-fill');
    const label    = document.getElementById('progress-label');

    progress.classList.add('show');

    let done = 0;
    const total = files.length;

    for (const file of files) {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('path', file.webkitRelativePath || file.name);

        await fetch('/upload', {
            method: 'POST',
            body: formData,
        });

        done++;
        const pct = Math.floor((done / total) * 100);
        fill.style.width = pct + '%';
        label.textContent = `アップロード中... ${done} / ${total} (${pct}%)`;
    }

    // 全ファイル送信後にzip化をGoに依頼
    const folderName = files[0].webkitRelativePath.split('/')[0];
    await fetch('/finalize?folder=' + encodeURIComponent(folderName) + '&username=' + encodeURIComponent(currentUser), {
        method: 'POST',
    });
    setTimeout(() => {
        progress.classList.remove('show');
        fill.style.width = '0%';
        showToast('✅ ' + folderName + '.zip をアップロードしました', 'success');
        fetchFiles();
    }, 400);
}

// ─── Fetch Files from Go ───
async function fetchFiles() {
    try {
        const res = await fetch('/files');
        const names = await res.json();
        renderFiles(names);
    } catch (e) {
        showToast('❌ ファイル一覧の取得に失敗しました', 'error');
    }
}

// ─── Render Files ───
function renderFiles(files) {
    const list = document.getElementById('file-list');
    document.getElementById('file-count').textContent = files ? files.length : 0;

    if (!files || files.length === 0) {
        list.innerHTML = `
            <div class="empty-state">
                <div class="empty-icon">🗂️</div>
                <p>// まだファイルがありません</p>
            </div>
        `;
        return;
    }

    const isTeacher = document.body.classList.contains('is-teacher');

    list.innerHTML = files.map((f, i) => `
        <div class="file-item" style="animation-delay:${i * 0.05}s">
            <div class="file-ext zip">${getExt(f.filename)}</div>
            <div class="file-info">
                <div class="file-name">${f.filename}</div>
                <div class="file-meta">
                    <span>📤 ${f.username}</span>
                    <span>📅 ${f.uploaded_at}</span>
                </div>
            </div>
            <div class="file-actions">
                <button class="action-btn dl" onclick="downloadFile('${f.filename}')">↓ DL</button>
                ${isTeacher ? `
                <button class="action-btn run" onclick="runFile('${f.filename}')">▶ 実行</button>
                <button class="action-btn del" onclick="deleteFile('${f.filename}')">✕ 削除</button>
                ` : ''}
            </div>
        </div>
    `).join('');
}

// ─── Download ───
function downloadFile(name) {
    const a = document.createElement('a');
    a.href = '/download?name=' + encodeURIComponent(name);
    a.download = name;
    a.click();
}

async function runFile(name) {
    showToast('▶ ' + name + ' を実行中...', 'success');

    const res = await fetch('/run?name=' + encodeURIComponent(name), {
        method: 'POST',
    });

    const data = await res.json();

    if (!res.ok) {
        showToast('❌ 実行に失敗しました', 'error');
        return;
    }

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


async function deleteFile(name) {
    const res = await fetch('/delete?name=' + encodeURIComponent(name), {
        method: 'DELETE',
    });
    if (res.ok) {
        showToast('🗑️ ' + name + ' を削除しました', 'error');
        fetchFiles();
    } else {
        showToast('❌ 削除に失敗しました', 'error');
    }
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

function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1024 / 1024).toFixed(1) + ' MB';
}