requireAuth();

/* ── Guard: redirect non-admins ─────────────────────────────────────────────── */
const currentUser = getUser();
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
  const active   = users.filter(u => u.subscription_status === 'active' || u.subscription_status === 'free').length;
  const trial    = users.filter(u => u.subscription_status === 'trialing').length;
  const inactive = total - active - trial;

  document.getElementById('adminStats').innerHTML = `
    <div class="admin-stat-card">
      <div class="admin-stat-num">${total}</div>
      <div class="admin-stat-label">Total Users</div>
    </div>
    <div class="admin-stat-card">
      <div class="admin-stat-num" style="color:var(--success)">${active}</div>
      <div class="admin-stat-label">Active / Free</div>
    </div>
    <div class="admin-stat-card">
      <div class="admin-stat-num" style="color:var(--warning)">${trial}</div>
      <div class="admin-stat-label">Trial</div>
    </div>
    <div class="admin-stat-card">
      <div class="admin-stat-num" style="color:var(--danger)">${inactive}</div>
      <div class="admin-stat-label">Inactive</div>
    </div>
  `;
}

/* ── Subscription badge ──────────────────────────────────────────────────────── */
function subBadge(status, trialEndsAt) {
  const map = {
    trialing:  ['badge-yellow',  '⏰ Trial'],
    active:    ['badge-green',   '✓ Active'],
    past_due:  ['badge-yellow',  '⚠ Past Due'],
    cancelled: ['badge-danger',  '✕ Cancelled'],
    suspended: ['badge-danger',  '🚫 Revoked'],
    free:      ['badge-purple',  '★ Free'],
    '':        ['badge-yellow',  'No Plan'],
  };
  const [cls, label] = map[status] || ['badge-yellow', status || 'None'];
  let extra = '';
  if ((status === 'trialing' || status === 'cancelled') && trialEndsAt) {
    extra = ` · trial ends ${trialEndsAt}`;
  }
  return `<span class="badge ${cls}">${label}${extra}</span>`;
}

/* ── User list ───────────────────────────────────────────────────────────────── */
function renderUsers() {
  const container = document.getElementById('usersContainer');
  if (users.length === 0) {
    container.innerHTML = '<p style="text-align:center;color:var(--text-3);padding:40px 0;">No users found.</p>';
    return;
  }
  container.innerHTML = `<div class="admin-user-list">${users.map(u => userRow(u)).join('')}</div>`;
}

function userRow(u) {
  const isSelf = u.id === currentUser.id;
  const status = u.subscription_status || '';

  let actions = '';
  if (isSelf) {
    actions = `<span class="badge badge-purple" title="Your account">You</span>`;
  } else {
    const btns = [];
    if (status !== 'free')      btns.push(`<button class="btn btn-sm btn-success"   onclick="setSub('${u.id}','free')">Grant Free</button>`);
    if (status !== 'trialing')  btns.push(`<button class="btn btn-sm btn-secondary" onclick="setSub('${u.id}','trialing')">Set Trial</button>`);
    if (status === 'suspended') btns.push(`<button class="btn btn-sm btn-success"   onclick="setSub('${u.id}','active')">Restore</button>`);
    else                        btns.push(`<button class="btn btn-sm btn-danger"    onclick="setSub('${u.id}','suspended')">Revoke</button>`);
    actions = btns.join('');
  }

  return `
    <div class="admin-user-row admin-user-row--sub" id="user-${u.id}">
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
        ${subBadge(status, u.trial_ends_at)}
      </div>
      <div class="admin-user-action admin-user-action--multi">
        ${actions}
      </div>
    </div>
  `;
}

/* ── Set subscription ────────────────────────────────────────────────────────── */
async function setSub(id, newStatus) {
  const row = document.getElementById('user-' + id);
  const btns = row ? row.querySelectorAll('button') : [];
  btns.forEach(b => { b.disabled = true; });

  try {
    await API.patch('/api/admin/users/' + id + '/subscription', { status: newStatus });
    const u = users.find(u => u.id === id);
    if (u) {
      u.subscription_status = newStatus;
      u.approved = ['trialing','active','free','past_due','cancelled'].includes(newStatus);
      if (newStatus === 'trialing') {
        const d = new Date(); d.setDate(d.getDate() + 7);
        u.trial_ends_at = d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
      } else {
        u.trial_ends_at = null;
      }
    }
    renderStats();
    renderUsers();
  } catch (err) {
    alert('Error: ' + err.message);
    btns.forEach(b => { b.disabled = false; });
  }
}

/* ── Boot ────────────────────────────────────────────────────────────────────── */
loadUsers();
