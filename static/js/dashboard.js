requireAuth();

/* ── State ──────────────────────────────────────────────────────────────────── */
let selectedLang  = null;
let selectedTopic = null;
let languages     = [];
let topics        = [];

/* ── Greeting ───────────────────────────────────────────────────────────────── */
(function initGreeting() {
  const hour = new Date().getHours();
  const time = hour < 12 ? 'morning' : hour < 17 ? 'afternoon' : 'evening';
  document.getElementById('greeting-time').textContent = time;

  const user = JSON.parse(localStorage.getItem('user') || '{}');
  const name = user.username || 'learner';
  document.getElementById('greeting-name').textContent = name;

  // Nav avatar
  const avatar = document.getElementById('navAvatar');
  avatar.textContent = name.charAt(0).toUpperCase();
  document.getElementById('navUsername').textContent = name;
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

  if (!selectedLang || !selectedTopic) {
    bar.classList.remove('visible');
    return;
  }

  const lang  = languages.find(l => l.code === selectedLang);
  const topic = topics.find(t => t.id === selectedTopic);

  info.innerHTML = `
    <span class="badge badge-purple">${lang.flag} ${lang.name}</span>
    <span style="color:var(--text-3)">·</span>
    <span class="badge badge-blue">${topic.icon} ${topic.name}</span>
  `;
  bar.classList.add('visible');
}

/* ── Start conversation ─────────────────────────────────────────────────────── */
async function startConversation() {
  if (!selectedLang || !selectedTopic) return;

  const btn = document.getElementById('startBtn');
  btn.disabled = true;
  btn.textContent = 'Starting…';

  try {
    const data = await API.post('/api/conversation/start', {
      language: selectedLang,
      topic:    selectedTopic,
    });

    const params = new URLSearchParams({
      session:   data.session_id,
      language:  data.language,
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
loadData();
