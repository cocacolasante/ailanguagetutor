requireAuth();

/* ── Guard: redirect non-admins ─────────────────────────────────────────────── */
const currentUser = JSON.parse(localStorage.getItem('user') || '{}');
if (!currentUser.is_admin) {
  window.location.href = '/dashboard.html';
}

/* ── Nav init ────────────────────────────────────────────────────────────────── */
(function initNav() {
  const name = currentUser.username || 'admin';
  document.getElementById('navAvatar').textContent = name.charAt(0).toUpperCase();
  document.getElementById('navUsername').textContent = name;
})();

/* ── State ───────────────────────────────────────────────────────────────────── */
let users = [];

/* ── Load users ──────────────────────────────────────────────────────────────── */
async function loadUsers() {
  try {
    const data = await API.get('/api/admin/users');
    users = data.users || [];
    renderStats();
    renderUsers();
  } catch (err) {
    document.getElementById('usersContainer').innerHTML =
      `<p style="color:var(--danger);text-align:center;padding:40px 0;">Failed to load users: ${err.message}</p>`;
  }
}

/* ── Stats ───────────────────────────────────────────────────────────────────── */
function renderStats() {
  const total    = users.length;
  const approved = users.filter(u => u.approved).length;
  const pending  = total - approved;

  document.getElementById('adminStats').innerHTML = `
    <div class="admin-stat-card">
      <div class="admin-stat-num">${total}</div>
      <div class="admin-stat-label">Total Users</div>
    </div>
    <div class="admin-stat-card">
      <div class="admin-stat-num" style="color:var(--success)">${approved}</div>
      <div class="admin-stat-label">Approved</div>
    </div>
    <div class="admin-stat-card">
      <div class="admin-stat-num" style="color:var(--warning)">${pending}</div>
      <div class="admin-stat-label">Pending</div>
    </div>
  `;
}

/* ── User list ───────────────────────────────────────────────────────────────── */
function renderUsers() {
  const container = document.getElementById('usersContainer');

  if (users.length === 0) {
    container.innerHTML = '<p style="text-align:center;color:var(--text-3);padding:40px 0;">No users found.</p>';
    return;
  }

  container.innerHTML = `
    <div class="admin-user-list">
      ${users.map(u => userRow(u)).join('')}
    </div>
  `;
}

function userRow(u) {
  const isSelf    = u.id === currentUser.id;
  const statusCls = u.approved ? 'badge-green' : 'badge-yellow';
  const statusTxt = u.approved ? 'Approved' : 'Pending';

  const actionBtn = isSelf
    ? `<span class="badge badge-purple" title="Your account">You</span>`
    : u.approved
      ? `<button class="btn btn-sm btn-danger" onclick="setApproval('${u.id}', false)">Revoke</button>`
      : `<button class="btn btn-sm btn-success" onclick="setApproval('${u.id}', true)">Approve</button>`;

  return `
    <div class="admin-user-row" id="user-${u.id}">
      <div class="admin-user-avatar">${u.username.charAt(0).toUpperCase()}</div>
      <div class="admin-user-info">
        <div class="admin-user-name">
          ${u.username}
          ${u.is_admin ? '<span class="badge badge-purple" style="font-size:0.65rem">Admin</span>' : ''}
        </div>
        <div class="admin-user-email">${u.email}</div>
        <div class="admin-user-joined">Joined ${u.created_at}</div>
      </div>
      <div class="admin-user-status">
        <span class="badge ${statusCls}">${statusTxt}</span>
      </div>
      <div class="admin-user-action">
        ${actionBtn}
      </div>
    </div>
  `;
}

/* ── Toggle approval ─────────────────────────────────────────────────────────── */
async function setApproval(id, approved) {
  // Use getElementById to avoid CSS selector issues with UUID-formatted IDs
  const row = document.getElementById('user-' + id);
  const btn = row ? row.querySelector('button') : null;

  try {
    if (btn) { btn.disabled = true; btn.textContent = '…'; }

    await API.patch('/api/admin/users/' + id + '/approval', { approved });

    // Update local state and re-render
    const u = users.find(u => u.id === id);
    if (u) u.approved = approved;
    renderStats();
    renderUsers();
  } catch (err) {
    alert('Error: ' + err.message);
    if (btn) { btn.disabled = false; btn.textContent = approved ? 'Approve' : 'Revoke'; }
  }
}

/* ── Boot ────────────────────────────────────────────────────────────────────── */
loadUsers();
