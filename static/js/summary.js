requireAuth();

const params   = new URLSearchParams(window.location.search);
const recordId = params.get('record');

if (!recordId) {
  window.location.href = '/dashboard.html';
}

const LANG_META = {
  it: { flag: '🇮🇹', name: 'Italian' },
  es: { flag: '🇪🇸', name: 'Spanish' },
  pt: { flag: '🇧🇷', name: 'Portuguese' },
  fr: { flag: '🇫🇷', name: 'French' },
  de: { flag: '🇩🇪', name: 'German' },
  ja: { flag: '🇯🇵', name: 'Japanese' },
  ru: { flag: '🇷🇺', name: 'Russian' },
  ro: { flag: '🇷🇴', name: 'Romanian' },
  zh: { flag: '🇨🇳', name: 'Chinese' },
};

const LEVEL_NAMES = ['', 'Beginner', 'Elementary', 'Intermediate', 'Advanced', 'Fluent'];

const BADGE_META = {
  streak_3:      { name: '3-Day Streak',      icon: '🔥' },
  streak_7:      { name: 'Week Warrior',       icon: '🔥' },
  streak_30:     { name: 'Monthly Master',     icon: '🌟' },
  streak_100:    { name: 'Century Streak',     icon: '💎' },
  first_conv:    { name: 'First Steps',        icon: '👶' },
  conv_10:       { name: 'Getting Started',    icon: '📖' },
  conv_50:       { name: 'Dedicated Learner',  icon: '🎓' },
  conv_100:      { name: 'Language Champion',  icon: '🏆' },
  fp_100:        { name: 'FP Collector',       icon: '⭐' },
  fp_500:        { name: 'FP Enthusiast',      icon: '🌟' },
  fp_1000:       { name: 'FP Expert',          icon: '💫' },
  fp_5000:       { name: 'FP Legend',          icon: '✨' },
  lang_level_5:  { name: 'Intermediate',       icon: '📈' },
  lang_level_10: { name: 'Advanced',           icon: '🎯' },
  lang_level_20: { name: 'Master',             icon: '👑' },
};

function formatDuration(secs) {
  if (!secs) return '—';
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  if (m === 0) return `${s}s`;
  return `${m}m ${s}s`;
}

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function renderList(items, emptyMsg) {
  if (!items || items.length === 0) return `<p class="summary-empty">${emptyMsg}</p>`;
  return `<ul class="summary-list">${items.map(i => `<li>${escapeHtml(i)}</li>`).join('')}</ul>`;
}

async function loadSummary() {
  // The record data may have been passed via sessionStorage from the end endpoint
  // or we fetch it directly
  let record = null;

  // Try sessionStorage first (set by conversation.js after end)
  const cached = sessionStorage.getItem('summary_record_' + recordId);
  if (cached) {
    try { record = JSON.parse(cached); } catch (_) {}
    sessionStorage.removeItem('summary_record_' + recordId);
  }

  // Fall back to API fetch
  if (!record) {
    try {
      record = await API.get(`/api/conversation/records/${recordId}`);
    } catch (err) {
      document.getElementById('summaryPage').innerHTML = `
        <div class="summary-error">
          <p>Could not load summary. <a href="/dashboard.html">Return to dashboard →</a></p>
        </div>`;
      return;
    }
  }

  renderSummary(record);
}

function renderSummary(r) {
  const lang     = LANG_META[r.language] || { flag: '🌐', name: r.language };
  const levelName = LEVEL_NAMES[r.level] || 'Intermediate';
  const duration = formatDuration(r.duration_secs);

  // Personality label
  const PERSONALITY_NAMES = {
    professor:          '🎓 Professor',
    'friendly-partner': '😊 Friendly Partner',
    bartender:          '🍺 Bartender',
    'business-executive': '💼 Business Executive',
    'travel-guide':     '🗺️ Travel Guide',
  };
  const personalityLabel = r.personality ? PERSONALITY_NAMES[r.personality] || r.personality : null;

  const page = document.getElementById('summaryPage');
  page.innerHTML = `
    <!-- Hero -->
    <div class="summary-hero">
      <div class="summary-hero-icon">🎉</div>
      <h1>Session Complete!</h1>
      <p class="summary-hero-sub">Great work practicing your ${lang.name}.</p>

      <!-- Meta badges -->
      <div class="summary-meta-badges">
        <span class="badge badge-purple">${lang.flag} ${lang.name}</span>
        <span class="badge badge-green">Level ${r.level} — ${levelName}</span>
        <span class="badge badge-blue">${r.topic_name || r.topic}</span>
        ${personalityLabel ? `<span class="badge badge-yellow">${personalityLabel}</span>` : ''}
      </div>

      <!-- Stats strip -->
      <div class="summary-stats-strip">
        <div class="summary-stat">
          <div class="summary-stat-val">⏱ ${duration}</div>
          <div class="summary-stat-lbl">Duration</div>
        </div>
        <div class="summary-stat">
          <div class="summary-stat-val">💬 ${r.message_count}</div>
          <div class="summary-stat-lbl">Messages</div>
        </div>
        <div class="summary-stat summary-stat-fp">
          <div class="summary-stat-val">+${r.fp_earned} FP</div>
          <div class="summary-stat-lbl">Earned</div>
        </div>
      </div>
    </div>

    <!-- Summary text -->
    <div class="summary-card">
      <h3 class="summary-section-title">📝 Session Overview</h3>
      <p class="summary-text">${escapeHtml(r.summary)}</p>
    </div>

    <!-- Content grid -->
    <div class="summary-grid">
      <div class="summary-card">
        <h3 class="summary-section-title">💬 Topics Discussed</h3>
        ${renderList(r.topics_discussed, 'No topics recorded.')}
      </div>
      <div class="summary-card">
        <h3 class="summary-section-title">📚 Vocabulary Learned</h3>
        ${renderList(r.vocabulary_learned, 'Keep practicing to build vocabulary!')}
      </div>
      <div class="summary-card">
        <h3 class="summary-section-title">✏️ Grammar Corrections</h3>
        ${renderList(r.grammar_corrections, 'No major corrections — great job!')}
      </div>
      <div class="summary-card">
        <h3 class="summary-section-title">🎯 Suggested Next Steps</h3>
        ${renderList(r.suggested_next_lessons, 'Keep practicing regularly!')}
      </div>
    </div>

    <!-- Actions -->
    <div class="summary-actions">
      <a href="/dashboard.html" class="btn btn-secondary btn-lg">← Return to Dashboard</a>
      <a
        href="/conversation.html?${buildRestartParams(r)}"
        class="btn btn-primary btn-lg"
        onclick="return startNewSession(event, '${r.language}', ${r.level}, '${r.topic}', '${r.personality || ''}')"
      >
        Practice Again →
      </a>
    </div>
  `;
}

function buildRestartParams(r) {
  return new URLSearchParams({
    language:    r.language,
    level:       r.level,
    topic:       r.topic,
    topicName:   r.topic_name || r.topic,
    personality: r.personality || '',
  }).toString();
}

async function startNewSession(event, language, level, topic, personality) {
  event.preventDefault();
  try {
    const data = await API.post('/api/conversation/start', {
      language, level, topic, personality: personality || '',
    });
    if (!data) return false;
    const p = new URLSearchParams({
      session:     data.session_id,
      language:    data.language,
      level:       data.level,
      topic:       data.topic,
      topicName:   data.topic_name,
      personality: data.personality || '',
    });
    window.location.href = `/conversation.html?${p}`;
  } catch (err) {
    console.error('Failed to start session:', err);
    alert('Failed to start session. Please try from the dashboard.');
  }
  return false;
}

loadSummary();
