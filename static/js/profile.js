requireAuth();

const user = getUser();

/* ── Nav init ────────────────────────────────────────────────────────────────── */
(function initNav() {
  const name = user.username || '?';
  document.getElementById('navAvatar').textContent = name.charAt(0).toUpperCase();
  document.getElementById('navUsername').textContent = name;
})();

/* ── Account card ────────────────────────────────────────────────────────────── */
document.getElementById('profileUsername').textContent = user.username || '—';
document.getElementById('profileEmail').textContent    = user.email    || '—';

if (user.created_at) {
  document.getElementById('profileJoined').textContent = new Date(user.created_at).toLocaleDateString();
}

/* ── Load billing status (live from server) ──────────────────────────────────── */
async function loadStatus() {
  try {
    const data = await API.get('/api/billing/status');
    renderSubCard(data);
  } catch (err) {
    document.getElementById('subCard').innerHTML =
      `<p style="color:var(--danger);padding:20px 0;">Failed to load subscription info.</p>`;
  }
}

/* ── Subscription card ───────────────────────────────────────────────────────── */
function renderSubCard(data) {
  const status      = data.subscription_status || 'none';
  const fullAccess  = data.has_full_access;
  const convAccess  = data.has_conversation_access;
  const trialEndsAt = data.trial_ends_at ? new Date(data.trial_ends_at) : null;

  const labels = {
    trialing:  { text: 'Free Trial',      cls: 'badge-yellow' },
    active:    { text: 'Active',          cls: 'badge-green'  },
    past_due:  { text: 'Past Due',        cls: 'badge-yellow' },
    cancelled: { text: 'Cancelled',       cls: 'badge-yellow' },
    suspended: { text: 'Suspended',       cls: 'badge-yellow' },
    free:      { text: 'Free (Admin)',    cls: 'badge-purple' },
    none:      { text: 'No Subscription', cls: 'badge-yellow' },
  };
  const badge = labels[status] || labels.none;

  // Trial / cancelled-in-trial row
  let trialRow = '';
  if (trialEndsAt) {
    const days = Math.max(0, Math.ceil((trialEndsAt - Date.now()) / 86400000));
    const label = status === 'cancelled' ? 'Access until' : 'Trial ends';
    trialRow = `
      <div class="profile-row">
        <span class="profile-label">${label}</span>
        <span class="profile-value">${trialEndsAt.toLocaleDateString()} (${days} day${days !== 1 ? 's' : ''} left)</span>
      </div>`;
  }

  // Access level row
  let accessText;
  if (fullAccess)                             accessText = 'All levels (1–5)';
  else if (status === 'trialing')             accessText = 'Trial — levels 1–3 only';
  else if (status === 'cancelled' && convAccess) accessText = 'Trial — levels 1–3 only (cancelled)';
  else                                        accessText = 'No access';

  const accessRow = `
    <div class="profile-row">
      <span class="profile-label">Access level</span>
      <span class="profile-value">${accessText}</span>
    </div>`;

  // Actions
  let actions = '';
  if (status === 'trialing') {
    actions = `
      <div class="profile-actions">
        <button class="btn btn-primary"   onclick="openPortal()">Upgrade to Full Access</button>
        <button class="btn btn-secondary" onclick="openPortal()">Manage Billing</button>
        <button class="btn btn-danger"    onclick="confirmCancel()">Cancel Subscription</button>
      </div>`;
  } else if (status === 'active' || status === 'past_due') {
    actions = `
      <div class="profile-actions">
        <button class="btn btn-secondary" onclick="openPortal()">Manage Billing &amp; Payment</button>
        <button class="btn btn-danger"    onclick="confirmCancel()">Cancel Subscription</button>
      </div>`;
  } else if (status === 'cancelled') {
    const note = convAccess
      ? `<p style="color:var(--text-3);font-size:0.85rem;margin-bottom:12px;">Your subscription is cancelled. You have access until your trial period ends.</p>`
      : `<p style="color:var(--danger);font-size:0.85rem;margin-bottom:12px;">Your subscription has ended.</p>`;
    actions = `
      <div class="profile-actions">
        ${note}
        <button class="btn btn-primary" onclick="resubscribe()">Resubscribe — $100/mo</button>
      </div>`;
  } else if (status === 'none') {
    actions = `
      <div class="profile-actions">
        <button class="btn btn-primary" onclick="resubscribe()">Subscribe — $100/mo</button>
      </div>`;
  } else if (status === 'suspended') {
    actions = `<p style="color:var(--danger);margin-top:12px;">Your account has been suspended. Please contact support.</p>`;
  }

  document.getElementById('subCard').innerHTML = `
    <div class="profile-row">
      <span class="profile-label">Plan</span>
      <span class="profile-value"><span class="badge ${badge.cls}">${badge.text}</span></span>
    </div>
    ${trialRow}
    ${accessRow}
    <div class="profile-row">
      <span class="profile-label">Price</span>
      <span class="profile-value">${status === 'free' ? 'Complimentary' : '$100 / month'}</span>
    </div>
    ${actions}
  `;

  // Show expired banner if redirected from dashboard
  if (new URLSearchParams(location.search).get('expired') === 'true' && !convAccess) {
    const card = document.getElementById('subCard');
    const notice = document.createElement('div');
    notice.style.cssText = 'background:rgba(239,68,68,0.1);border:1px solid var(--danger);border-radius:8px;padding:12px 16px;margin-bottom:12px;color:var(--danger);font-size:0.9rem;';
    notice.textContent = 'Your trial has ended. Please resubscribe to continue practicing.';
    card.prepend(notice);
  }
}

/* ── Cancel subscription ─────────────────────────────────────────────────────── */
async function confirmCancel() {
  if (!confirm('Cancel your subscription?\n\nIf you are in a trial, you keep access until the trial ends. After that, you can resubscribe at any time.')) return;
  try {
    await API.post('/api/billing/cancel', {});
    loadStatus(); // refresh card
    // Also update cached user object
    const u = getUser();
    u.subscription_status = 'cancelled';
    localStorage.setItem('user', JSON.stringify(u));
  } catch (err) {
    alert('Error cancelling subscription: ' + err.message);
  }
}

/* ── Open Stripe Customer Portal ─────────────────────────────────────────────── */
async function openPortal() {
  try {
    const data = await API.post('/api/billing/portal', {});
    window.location.href = data.portal_url;
  } catch (err) {
    alert('Error opening billing portal: ' + err.message);
  }
}

/* ── Resubscribe (create new checkout) ───────────────────────────────────────── */
async function resubscribe() {
  try {
    const data = await API.post('/api/billing/checkout', { plan: 'immediate' });
    window.location.href = data.checkout_url;
  } catch (err) {
    alert('Error starting checkout: ' + err.message);
  }
}

/* ── Learning Preferences ────────────────────────────────────────────────────── */

const PREF_LANGUAGES = [
  { code: 'it', flag: '🇮🇹', name: 'Italian' },
  { code: 'es', flag: '🇪🇸', name: 'Spanish' },
  { code: 'pt', flag: '🇧🇷', name: 'Portuguese' },
  // { code: 'fr', flag: '🇫🇷', name: 'French' },
  // { code: 'de', flag: '🇩🇪', name: 'German' },
  // { code: 'ja', flag: '🇯🇵', name: 'Japanese' },
  // { code: 'ru', flag: '🇷🇺', name: 'Russian' },
  // { code: 'ro', flag: '🇷🇴', name: 'Romanian' },
  // { code: 'zh', flag: '🇨🇳', name: 'Chinese' },
];

const PREF_LEVELS = [
  { value: 1, label: '1', name: 'Beginner',           desc: 'Learning the basics' },
  { value: 2, label: '2', name: 'Elementary',         desc: 'Simple conversations' },
  { value: 3, label: '3', name: 'Intermediate',       desc: 'Everyday topics' },
  { value: 4, label: '4', name: 'Upper-Intermediate', desc: 'Complex discussions' },
  { value: 5, label: '5', name: 'Fluent',             desc: 'Near-native' },
];

const PREF_PERSONALITIES = [
  { id: '',                   icon: '🤷', name: 'No Preference',     desc: 'Balanced, adaptive tutor' },
  { id: 'professor',          icon: '🎓', name: 'The Professor',     desc: 'Structured and academic' },
  { id: 'friendly-partner',   icon: '😊', name: 'Friendly Partner',  desc: 'Casual and encouraging' },
  { id: 'bartender',          icon: '🍺', name: 'The Bartender',     desc: 'Relaxed pub-style chat' },
  { id: 'business-executive', icon: '💼', name: 'Business Executive', desc: 'Formal and professional' },
  { id: 'travel-guide',       icon: '🗺️', name: 'Travel Guide',     desc: 'Adventure and exploration' },
];

let prefLang        = '';
let prefLevel       = 0;
let prefPersonality = '';

async function initPreferences() {
  // Fetch fresh user data to get latest pref values
  let currentUser = user;
  try {
    currentUser = await API.get('/api/auth/me');
  } catch {}

  prefLang        = currentUser.pref_language    || '';
  prefLevel       = currentUser.pref_level       || 0;
  prefPersonality = currentUser.pref_personality || '';

  // Onboarding hint
  if (new URLSearchParams(location.search).get('onboarding') === '1') {
    document.getElementById('onboardingHint')?.classList.remove('hidden');
  }

  renderPrefLanguages();
  renderPrefLevels();
  renderPrefPersonalities();
}

function renderPrefLanguages() {
  const grid = document.getElementById('prefLangGrid');
  if (!grid) return;
  grid.innerHTML = PREF_LANGUAGES.map(l => `
    <div class="pref-lang-card${prefLang === l.code ? ' selected' : ''}"
         id="pref-lang-${l.code}"
         onclick="selectPrefLang('${l.code}')">
      <span>${l.flag}</span>
      <span>${l.name}</span>
    </div>`).join('');
}

function renderPrefLevels() {
  const grid = document.getElementById('prefLevelGrid');
  if (!grid) return;
  grid.innerHTML = PREF_LEVELS.map(l => `
    <div class="pref-level-card${prefLevel === l.value ? ' selected' : ''}"
         id="pref-level-${l.value}"
         onclick="selectPrefLevel(${l.value})">
      <div class="pref-level-num">${l.label}</div>
      <div class="pref-level-name">${l.name}</div>
    </div>`).join('');
}

function renderPrefPersonalities() {
  const grid = document.getElementById('prefPersonalityGrid');
  if (!grid) return;
  grid.innerHTML = PREF_PERSONALITIES.map(p => `
    <div class="pref-personality-card${prefPersonality === p.id ? ' selected' : ''}"
         id="pref-personality-${p.id || 'none'}"
         onclick="selectPrefPersonality('${p.id}')">
      <span>${p.icon}</span>
      <span>${p.name}</span>
    </div>`).join('');
}

function selectPrefLang(code) {
  prefLang = code;
  document.querySelectorAll('.pref-lang-card').forEach(c => c.classList.remove('selected'));
  document.getElementById(`pref-lang-${code}`)?.classList.add('selected');
}

function selectPrefLevel(value) {
  prefLevel = value;
  document.querySelectorAll('.pref-level-card').forEach(c => c.classList.remove('selected'));
  document.getElementById(`pref-level-${value}`)?.classList.add('selected');
}

function selectPrefPersonality(id) {
  prefPersonality = id;
  document.querySelectorAll('.pref-personality-card').forEach(c => c.classList.remove('selected'));
  document.getElementById(`pref-personality-${id || 'none'}`)?.classList.add('selected');
}

async function savePreferences() {
  const statusEl = document.getElementById('prefSaveStatus');
  try {
    await API.patch('/api/user/preferences', {
      language:    prefLang,
      level:       prefLevel,
      personality: prefPersonality,
    });
    if (statusEl) {
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 3000);
    }
  } catch (err) {
    alert('Failed to save preferences: ' + (err.message || 'Unknown error'));
  }
}

/* ── Boot ────────────────────────────────────────────────────────────────────── */
loadStatus();
initPreferences();
