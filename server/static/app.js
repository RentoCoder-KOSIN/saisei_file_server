'use strict';

let currentRole = 'student';
let currentUser = '';

// ─── Role Select ───
function selectRole(btn) {
    document.querySelectorAll('.role-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    currentRole = btn.dataset.role;
}

// ─── Login ───
function login() {
    const username = document.getElementById('login-username').value.trim();
    const password = document.getElementById('login-password').value;

    if (!username || !password) {
        showToast('❌ IDとパスワードを入力してください', 'error');
        return;
    }

    currentUser = username;

    const badge = document.getElementById('user-badge');
    if (currentRole === 'teacher') {
        badge.textContent = '👨‍🏫 ' + username;
        badge.className = 'user-badge teacher';
        document.body.classList.add('is-teacher');
    } else {
        badge.textContent = '👨‍🎓 ' + username;
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
    await fetch('/finalize?folder=' + encodeURIComponent(folderName), {
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
function renderFiles(names) {
    const list = document.getElementById('file-list');
    document.getElementById('file-count').textContent = names ? names.length : 0;

    if (!names || names.length === 0) {
        list.innerHTML = `
            <div class="empty-state">
                <div class="empty-icon">🗂️</div>
                <p>// まだファイルがありません</p>
            </div>
        `;
        return;
    }

    list.innerHTML = names.map((name, i) => `
        <div class="file-item" style="animation-delay:${i * 0.05}s">
            <div class="file-ext py">${getExt(name)}</div>
            <div class="file-info">
                <div class="file-name">${name}</div>
                <div class="file-meta">
                    <span>💾 ${name}</span>
                </div>
            </div>
            <div class="file-actions">
                <button class="action-btn dl" onclick="downloadFile('${name}')">↓ DL</button>
                <button class="action-btn run teacher-only" onclick="runFile('${name}')">▶ 実行</button>
                <button class="action-btn del teacher-only" onclick="deleteFile('${name}')">✕ 削除</button>
            </div>
        </div>
    `).join('');

    if (document.body.classList.contains('is-teacher')) {
        document.querySelectorAll('.teacher-only').forEach(el => {
            el.style.display = 'flex';
        });
    }
}

function downloadFile(name) {
    const a = document.createElement('a');
    a.href = '/download?name=' + encodeURIComponent(name);
    a.download = name;
    a.click();
}

// ─── Run / Delete ───
function runFile(name) {
    showToast('▶ ' + name + ' を実行中...', 'success');
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