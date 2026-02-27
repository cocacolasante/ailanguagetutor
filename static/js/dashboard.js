requireAuth();

/* ── State ──────────────────────────────────────────────────────────────────── */
let selectedLang  = null;
let selectedLevel = null;
let selectedTopic = null;
let languages     = [];
let topics        = [];
let subStatus     = '';

const levels = [
  { id: 1, label: 'Beginner',     emoji: '🌱', desc: 'New to the language — vocabulary, pronunciation & basics' },
  { id: 2, label: 'Elementary',   emoji: '📖', desc: 'Simple phrases & everyday grammar with guided support' },
  { id: 3, label: 'Intermediate', emoji: '💬', desc: 'Comfortable conversation on familiar topics' },
  { id: 4, label: 'Advanced',     emoji: '🎯', desc: 'Nuanced discussion, idioms & complex grammar' },
  { id: 5, label: 'Fluent',       emoji: '🚀', desc: 'Fully natural, native-speed conversation' },
];

/* ── Greeting ───────────────────────────────────────────────────────────────── */
(function initGreeting() {
  const hour = new Date().getHours();
  const time = hour < 12 ? 'morning' : hour < 17 ? 'afternoon' : 'evening';
  document.getElementById('greeting-time').textContent = time;

  const user = getUser();
  const name = user.username || 'learner';
  document.getElementById('greeting-name').textContent = name;

  // Nav avatar
  const avatar = document.getElementById('navAvatar');
  avatar.textContent = name.charAt(0).toUpperCase();
  document.getElementById('navUsername').textContent = name;

  // Show admin link only for admin users
  if (user.is_admin) {
    document.getElementById('adminLink').classList.remove('hidden');
  }

  // Subscription banners + access control
  subStatus = user.subscription_status || '';
  const trialEndsAt = user.trial_ends_at ? new Date(user.trial_ends_at) : null;
  const trialExpired = trialEndsAt && trialEndsAt <= new Date();

  // Cancelled + trial expired → redirect to profile (locked out of features)
  if (subStatus === 'cancelled' && trialExpired) {
    window.location.href = '/profile.html?expired=true';
    return;
  }

  if (subStatus === 'trialing') {
    const banner = document.getElementById('trialBanner');
    banner.classList.remove('hidden');
    let txt = '⏰ Trial active — levels 1–3 only.';
    if (trialEndsAt) {
      const days = Math.max(0, Math.ceil((trialEndsAt - Date.now()) / 86400000));
      txt = `⏰ Trial: ${days} day${days !== 1 ? 's' : ''} remaining · Levels 1–3 only.`;
    }
    document.getElementById('trialBannerText').textContent = txt;
  } else if (subStatus === 'cancelled') {
    // Cancelled but still in trial period
    const banner = document.getElementById('trialBanner');
    banner.classList.remove('hidden');
    const days = trialEndsAt ? Math.max(0, Math.ceil((trialEndsAt - Date.now()) / 86400000)) : 0;
    document.getElementById('trialBannerText').textContent =
      `⚠️ Subscription cancelled — trial access ends in ${days} day${days !== 1 ? 's' : ''}. Levels 1–3 only.`;
  } else if (subStatus === 'past_due') {
    document.getElementById('pastdueBanner').classList.remove('hidden');
  }
})();

/* ── Load data ──────────────────────────────────────────────────────────────── */
async function loadData() {
  try {
    [languages, topics] = await Promise.all([
      API.get('/api/languages'),
      API.get('/api/topics'),
    ]);
    renderLanguages();
    renderTopics();
  } catch (err) {
    console.error('Failed to load data:', err);
  }
}

/* ── Language cards ─────────────────────────────────────────────────────────── */
function renderLanguages() {
  const grid = document.getElementById('languageGrid');
  grid.innerHTML = languages.map(lang => `
    <div
      class="language-card"
      id="lang-${lang.code}"
      onclick="selectLanguage('${lang.code}')"
      role="radio"
      aria-checked="false"
      tabindex="0"
      onkeydown="if(event.key==='Enter'||event.key===' ')selectLanguage('${lang.code}')"
    >
      <div class="lang-check">✓</div>
      <div class="lang-flag">${lang.flag}</div>
      <div class="lang-name">${lang.name}</div>
      <div class="lang-native">${lang.native_name}</div>
    </div>
  `).join('');
}

function selectLanguage(code) {
  // Deselect previous
  if (selectedLang) {
    const prev = document.getElementById(`lang-${selectedLang}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked','false'); }
  }
  selectedLang = code;
  const card = document.getElementById(`lang-${code}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked','true'); }
  updateStartBar();
}

/* ── Level cards ────────────────────────────────────────────────────────────── */
function renderLevels() {
  const isTrial   = subStatus === 'trialing';
  const grid      = document.getElementById('levelGrid');
  grid.innerHTML = levels.map(lv => {
    const locked = isTrial && lv.id > 3;
    return `
    <div
      class="level-card${locked ? ' level-locked' : ''}"
      id="level-${lv.id}"
      onclick="${locked ? 'showUpgradePrompt()' : 'selectLevel(' + lv.id + ')'}"
      role="radio"
      aria-checked="false"
      tabindex="0"
      title="${locked ? 'Upgrade to unlock' : ''}"
    >
      <div class="level-check">✓</div>
      ${locked ? '<div class="level-lock">🔒</div>' : ''}
      <div class="level-num">${lv.id}</div>
      <div class="level-emoji">${lv.emoji}</div>
      <div class="level-label">${lv.label}</div>
      <div class="level-desc">${lv.desc}</div>
    </div>
  `}).join('');
}

function showUpgradePrompt() {
  if (confirm('Levels 4 and 5 require a full subscription ($100/mo).\n\nUpgrade now?')) {
    window.location.href = '/profile.html';
  }
}

function selectLevel(id) {
  if (selectedLevel) {
    const prev = document.getElementById(`level-${selectedLevel}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked','false'); }
  }
  selectedLevel = id;
  const card = document.getElementById(`level-${id}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked','true'); }
  updateStartBar();
}

/* ── Topic cards (grouped) ──────────────────────────────────────────────────── */
function renderTopics() {
  const container = document.getElementById('topicsContainer');

  // Group by category
  const categories = {};
  topics.forEach(t => {
    if (!categories[t.category]) categories[t.category] = [];
    categories[t.category].push(t);
  });

  container.innerHTML = Object.entries(categories).map(([cat, items]) => `
    <div class="category-section">
      <div class="category-title">${cat}</div>
      <div class="topic-grid">
        ${items.map(t => `
          <div
            class="topic-card"
            id="topic-${t.id}"
            onclick="selectTopic('${t.id}')"
            role="radio"
            aria-checked="false"
            tabindex="0"
            onkeydown="if(event.key==='Enter'||event.key===' ')selectTopic('${t.id}')"
          >
            <div class="topic-icon">${t.icon}</div>
            <div class="topic-name">${t.name}</div>
            <div class="topic-desc">${t.description}</div>
          </div>
        `).join('')}
      </div>
    </div>
  `).join('');
}

function selectTopic(id) {
  if (selectedTopic) {
    const prev = document.getElementById(`topic-${selectedTopic}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked','false'); }
  }
  selectedTopic = id;
  const card = document.getElementById(`topic-${id}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked','true'); }

  // Scroll topic into view gently
  card?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });

  updateStartBar();
}

/* ── Start bar ──────────────────────────────────────────────────────────────── */
function updateStartBar() {
  const bar  = document.getElementById('startBar');
  const info = document.getElementById('selectionInfo');

  if (!selectedLang || !selectedLevel || !selectedTopic) {
    bar.classList.remove('visible');
    return;
  }

  const lang  = languages.find(l => l.code === selectedLang);
  const lv    = levels.find(l => l.id === selectedLevel);
  const topic = topics.find(t => t.id === selectedTopic);

  info.innerHTML = `
    <span class="badge badge-purple">${lang.flag} ${lang.name}</span>
    <span style="color:var(--text-3)">·</span>
    <span class="badge badge-green">${lv.emoji} Level ${lv.id} — ${lv.label}</span>
    <span style="color:var(--text-3)">·</span>
    <span class="badge badge-blue">${topic.icon} ${topic.name}</span>
  `;
  bar.classList.add('visible');
}

/* ── Start conversation ─────────────────────────────────────────────────────── */
async function startConversation() {
  if (!selectedLang || !selectedLevel || !selectedTopic) return;

  const btn = document.getElementById('startBtn');
  btn.disabled = true;
  btn.textContent = 'Starting…';

  try {
    const data = await API.post('/api/conversation/start', {
      language: selectedLang,
      level:    selectedLevel,
      topic:    selectedTopic,
    });

    if (!data) return; // 401 already handled — token expired, navigating to login

    const params = new URLSearchParams({
      session:   data.session_id,
      language:  data.language,
      level:     selectedLevel,
      topic:     data.topic,
      topicName: data.topic_name,
    });
    window.location.href = `/conversation.html?${params}`;
  } catch (err) {
    console.error('Failed to start session:', err);
    btn.disabled = false;
    btn.textContent = 'Start Conversation →';
    alert('Failed to start session. Please try again.');
  }
}

/* ── Init ───────────────────────────────────────────────────────────────────── */
renderLevels();
loadData();
