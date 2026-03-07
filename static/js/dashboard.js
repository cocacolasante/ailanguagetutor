requireAuth();

/* ── State ──────────────────────────────────────────────────────────────────── */
let selectedLang        = null;
let selectedLevel       = null;
let selectedPersonality = null;
let selectedMode        = null;
let selectedTopic       = null;
let languages           = [];
let topics              = [];
let personalities       = [];
let subStatus           = '';
let userStats           = null;

// Extra languages revealed by "Load More"
const extraLanguages = [
  { code: 'fr', name: 'French',   native_name: 'Français',  flag: '🇫🇷' },
  { code: 'de', name: 'German',   native_name: 'Deutsch',   flag: '🇩🇪' },
  { code: 'ja', name: 'Japanese', native_name: '日本語',     flag: '🇯🇵' },
  { code: 'ru', name: 'Russian',  native_name: 'Русский',   flag: '🇷🇺' },
  { code: 'ro', name: 'Romanian', native_name: 'Română',    flag: '🇷🇴' },
  { code: 'zh', name: 'Chinese',  native_name: '中文',       flag: '🇨🇳' },
];
const extraLangCodes = new Set(extraLanguages.map(l => l.code));

const levels = [
  { id: 1, label: 'Beginner',     emoji: '🌱', desc: 'New to the language — vocabulary, pronunciation & basics' },
  { id: 2, label: 'Elementary',   emoji: '📖', desc: 'Simple phrases & everyday grammar with guided support' },
  { id: 3, label: 'Intermediate', emoji: '💬', desc: 'Comfortable conversation on familiar topics' },
  { id: 4, label: 'Advanced',     emoji: '🎯', desc: 'Nuanced discussion, idioms & complex grammar' },
  { id: 5, label: 'Fluent',       emoji: '🚀', desc: 'Fully natural, native-speed conversation' },
];

const MODES = [
  {
    id: 'conversational',
    icon: '💬',
    name: 'Conversational Practice',
    desc: 'Practice real conversations, role-play scenarios, and travel simulations.',
    comingSoon: false,
  },
  {
    id: 'grammar',
    icon: '📝',
    name: 'Grammar & Skills',
    desc: 'Vocabulary builder, sentence construction, pronunciation, listening, writing.',
    comingSoon: false,
  },
  {
    id: 'cultural',
    icon: '🌍',
    name: 'Cultural Language Learning',
    desc: 'Cultural context, stories, idioms, food culture, and history.',
    comingSoon: false,
  },
  {
    id: 'immersion',
    icon: '🔵',
    name: 'Immersion Mode',
    desc: 'Zero English. The AI speaks only your target language at native speed.',
    comingSoon: false,
  },
];

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

const LANG_FLAGS = {
  it:'🇮🇹', es:'🇪🇸', pt:'🇧🇷', fr:'🇫🇷', de:'🇩🇪',
  ja:'🇯🇵', ru:'🇷🇺', ro:'🇷🇴', zh:'🇨🇳',
};

/* ── Greeting + auth ────────────────────────────────────────────────────────── */
(function initGreeting() {
  const hour = new Date().getHours();
  const time = hour < 12 ? 'morning' : hour < 17 ? 'afternoon' : 'evening';
  document.getElementById('greeting-time').textContent = time;

  const user = getUser();
  const name = user.username || 'learner';
  document.getElementById('greeting-name').textContent = name;

  const avatar = document.getElementById('navAvatar');
  avatar.textContent = name.charAt(0).toUpperCase();
  document.getElementById('navUsername').textContent = name;

  if (user.is_admin) {
    document.getElementById('adminLink').classList.remove('hidden');
  }

  if (user.email_verified === false) {
    document.getElementById('emailVerifyBanner').classList.remove('hidden');
  }

  subStatus = user.subscription_status || '';
  const trialEndsAt = user.trial_ends_at ? new Date(user.trial_ends_at) : null;
  const trialExpired = trialEndsAt && trialEndsAt <= new Date();

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
    const banner = document.getElementById('trialBanner');
    banner.classList.remove('hidden');
    const days = trialEndsAt ? Math.max(0, Math.ceil((trialEndsAt - Date.now()) / 86400000)) : 0;
    document.getElementById('trialBannerText').textContent =
      `⚠️ Subscription cancelled — trial access ends in ${days} day${days !== 1 ? 's' : ''}. Levels 1–3 only.`;
  } else if (subStatus === 'past_due') {
    document.getElementById('pastdueBanner').classList.remove('hidden');
  }
})();

/* ── Load all data ──────────────────────────────────────────────────────────── */
async function loadData() {
  try {
    [languages, topics, personalities, userStats] = await Promise.all([
      API.get('/api/languages'),
      API.get('/api/topics'),
      API.get('/api/personalities'),
      API.get('/api/user/stats').catch(() => null),
    ]);
    renderLanguages();
    renderPersonalities();
    if (userStats) renderStats(userStats);
  } catch (err) {
    console.error('Failed to load data:', err);
  }
}

/* ── Stats widgets ──────────────────────────────────────────────────────────── */
function renderStats(stats) {
  document.getElementById('streakValue').textContent =
    stats.streak > 0 ? `${stats.streak}` : '0';
  document.getElementById('fpValue').textContent =
    stats.total_fp > 0 ? stats.total_fp.toLocaleString() : '0';
  document.getElementById('convCount').textContent =
    stats.conversation_count > 0 ? stats.conversation_count : '0';

  // Recent conversation widget
  const recent = stats.recent_conversations;
  if (recent && recent.length > 0) {
    const r = recent[0];
    const widget = document.getElementById('recentConvWidget');
    widget.style.display = '';
    const flag = LANG_FLAGS[r.language] || '🌐';
    const dur = r.duration_secs > 0
      ? `${Math.floor(r.duration_secs / 60)}m ${r.duration_secs % 60}s`
      : '—';

    document.getElementById('recentConvInfo').innerHTML = `
      <div class="recent-lang">${flag} ${capitalize(r.language)} · Level ${r.level}</div>
      <div class="recent-topic">${r.topic_name}</div>
      <div class="recent-dur">⏱ ${dur} · ${r.message_count} messages · +${r.fp_earned} FP</div>
      <div class="recent-summary">${escapeHtml(r.summary || '')}</div>
    `;
    document.getElementById('recentConvActions').innerHTML = `
      <a href="/summary.html?record=${r.id}" class="btn btn-secondary btn-sm">View Summary</a>
    `;
  }

  // Achievements
  const achievements = stats.achievements || [];
  if (achievements.length > 0) {
    const row = document.getElementById('achievementsRow');
    row.classList.remove('hidden');
    const list = document.getElementById('achievementsList');
    const toShow = achievements.slice(-5).reverse();
    list.innerHTML = toShow.map(id => {
      const b = BADGE_META[id] || { name: id, icon: '🏅' };
      return `<div class="achievement-pill" title="${b.name}">${b.icon} ${b.name}</div>`;
    }).join('');
  }
}

function capitalize(str) {
  return str ? str.charAt(0).toUpperCase() + str.slice(1) : str;
}

function escapeHtml(str) {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

/* ── Language cards ─────────────────────────────────────────────────────────── */
function langCardHTML(lang, isExtra) {
  return `
    <div
      class="language-card${isExtra ? ' lang-extra' : ''}"
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
    </div>`;
}

function renderLanguages() {
  const grid = document.getElementById('languageGrid');
  const primaryHTML = languages
    .filter(l => !extraLangCodes.has(l.code))
    .map(l => langCardHTML(l, false))
    .join('');
  const extraHTML = extraLanguages.map(l => langCardHTML(l, true)).join('');
  grid.innerHTML = primaryHTML + extraHTML;
  document.getElementById('langLoadMoreWrap').style.display = '';
}

function loadMoreLanguages() {
  document.querySelectorAll('.lang-extra').forEach(el => el.classList.remove('lang-extra'));
  document.getElementById('langLoadMoreWrap').style.display = 'none';
}

function selectLanguage(code) {
  if (selectedLang) {
    const prev = document.getElementById(`lang-${selectedLang}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }
  }
  selectedLang = code;
  const card = document.getElementById(`lang-${code}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked', 'true'); }
  updateStartBar();
}

/* ── Level cards ────────────────────────────────────────────────────────────── */
function renderLevels() {
  const isTrial = subStatus === 'trialing' || subStatus === 'cancelled';
  const grid = document.getElementById('levelGrid');
  grid.innerHTML = levels.map(lv => {
    const locked = isTrial && lv.id > 3;
    return `
    <div
      class="level-card${locked ? ' level-locked' : ''}"
      id="level-${lv.id}"
      onclick="${locked ? 'showUpgradePrompt()' : `selectLevel(${lv.id})`}"
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
    </div>`;
  }).join('');
}

function showUpgradePrompt() {
  if (confirm('Levels 4 and 5 require a full subscription.\n\nUpgrade now?')) {
    window.location.href = '/profile.html';
  }
}

function selectLevel(id) {
  if (selectedLevel) {
    const prev = document.getElementById(`level-${selectedLevel}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }
  }
  selectedLevel = id;
  const card = document.getElementById(`level-${id}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked', 'true'); }
  updateStartBar();
}

/* ── Personality cards ──────────────────────────────────────────────────────── */
function renderPersonalities() {
  const grid = document.getElementById('personalityGrid');
  const none = `
    <div
      class="personality-card selected"
      id="personality-none"
      onclick="selectPersonality('')"
      role="radio"
      aria-checked="true"
      tabindex="0"
    >
      <div class="personality-check">✓</div>
      <div class="personality-icon">🎲</div>
      <div class="personality-name">No Preference</div>
      <div class="personality-desc">Standard AI tutor — balanced and adaptive.</div>
    </div>`;

  const cards = personalities.map(p => `
    <div
      class="personality-card"
      id="personality-${p.id}"
      onclick="selectPersonality('${p.id}')"
      role="radio"
      aria-checked="false"
      tabindex="0"
    >
      <div class="personality-check">✓</div>
      <div class="personality-icon">${p.icon}</div>
      <div class="personality-name">${p.name}</div>
      <div class="personality-desc">${p.description}</div>
    </div>`).join('');

  grid.innerHTML = none + cards;
  selectedPersonality = '';
}

function selectPersonality(id) {
  const prevId = selectedPersonality === '' ? 'none' : selectedPersonality;
  const prev = document.getElementById(`personality-${prevId}`);
  if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }

  selectedPersonality = id;
  const newId = id === '' ? 'none' : id;
  const card = document.getElementById(`personality-${newId}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked', 'true'); }

  updateStartBar();
}

/* ── Learning Mode cards (Step 4) ───────────────────────────────────────────── */
function renderModes() {
  const grid = document.getElementById('learningModeGrid');
  grid.innerHTML = MODES.map(m => `
    <div
      class="learning-mode-card${m.comingSoon ? ' coming-soon' : ''}"
      id="mode-${m.id}"
      onclick="${m.comingSoon ? '' : `selectMode('${m.id}')`}"
      role="radio"
      aria-checked="false"
      tabindex="${m.comingSoon ? '-1' : '0'}"
    >
      <div class="learning-mode-icon">${m.icon}</div>
      <div class="learning-mode-name">${m.name}</div>
      <div class="learning-mode-desc">${m.desc}</div>
      ${m.comingSoon ? '<div class="coming-soon-badge">Coming Soon</div>' : ''}
    </div>
  `).join('');
}

function selectMode(id) {
  if (selectedMode) {
    const prev = document.getElementById(`mode-${selectedMode}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }
  }
  selectedMode = id;
  const card = document.getElementById(`mode-${id}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked', 'true'); }

  // Clear selected topic
  if (selectedTopic) {
    const prev = document.getElementById(`topic-${selectedTopic}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }
    selectedTopic = null;
  }

  renderActivitySection(id);
  updateStartBar();
}

/* ── Activity Section (Step 5) ──────────────────────────────────────────────── */
function renderActivitySection(mode) {
  const section = document.getElementById('activitySection');
  const container = document.getElementById('activityContainer');

  section.classList.remove('hidden');

  if (mode === 'conversational') {
    renderConversationalActivities(container);
  } else if (mode === 'grammar') {
    renderGrammarActivities(container);
  } else if (mode === 'cultural') {
    renderCulturalActivities(container);
  } else if (mode === 'immersion') {
    renderImmersionActivities(container);
  } else {
    const modeInfo = MODES.find(m => m.id === mode);
    renderComingSoonPanel(container, modeInfo);
  }

  section.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

const GRAMMAR_ACTIVITIES = [
  {
    id: 'grammar-vocabulary',
    icon: '📚',
    name: 'Vocabulary Builder',
    desc: 'Learn new words through guided exercises, phonetic breakdowns, and translation quizzes.',
    tag: 'Vocabulary',
  },
  {
    id: 'grammar-sentences',
    icon: '✏️',
    name: 'Sentence Construction',
    desc: 'Build grammatically correct sentences through word-order and fill-in-the-blank exercises.',
    tag: 'Grammar',
  },
  {
    id: 'grammar-pronunciation',
    icon: '🗣️',
    name: 'Pronunciation Practice',
    desc: 'Perfect your pronunciation with phonetic breakdowns, syllable stress guides, and sound drills.',
    tag: 'Pronunciation',
  },
  {
    id: 'grammar-listening',
    icon: '👂',
    name: 'Listening Comprehension',
    desc: 'Improve listening skills through short passages and targeted comprehension questions.',
    tag: 'Listening',
  },
  {
    id: 'grammar-writing',
    icon: '📝',
    name: 'Writing Coach',
    desc: 'Submit your writing for detailed grammar corrections, style upgrades, and encouragement.',
    tag: 'Writing',
  },
];

function renderGrammarActivities(container) {
  container.innerHTML = `
    <p class="activity-section-intro">Choose an exercise type. Each session is structured, adaptive, and ends with an AI performance summary.</p>
    <div class="grammar-grid">
      ${GRAMMAR_ACTIVITIES.map(a => `
        <div
          class="grammar-card"
          id="topic-${a.id}"
          onclick="selectTopic('${a.id}')"
          role="radio"
          aria-checked="false"
          tabindex="0"
          onkeydown="if(event.key==='Enter'||event.key===' ')selectTopic('${a.id}')"
        >
          <div class="grammar-card-header">
            <div class="grammar-card-icon">${a.icon}</div>
            <div class="grammar-card-tag">${a.tag}</div>
          </div>
          <div class="grammar-card-name">${a.name}</div>
          <div class="grammar-card-desc">${a.desc}</div>
        </div>
      `).join('')}
    </div>
  `;
}

const IMMERSION_SCENARIOS = [
  { id: 'immersion-daily',  icon: '🏡', name: 'Daily Life',      desc: 'Shopping, transport, home, errands — the full texture of everyday life.' },
  { id: 'immersion-social', icon: '🥂', name: 'Social Scene',    desc: 'A dinner party, night out, or casual get-together with native speakers.' },
  { id: 'immersion-work',   icon: '💼', name: 'Workplace',       desc: 'Meetings, colleagues, and office life entirely in the target language.' },
  { id: 'immersion-city',   icon: '🏙️', name: 'City Exploration', desc: 'Ask locals for help, navigate transit, and explore neighbourhoods.' },
  { id: 'immersion-media',  icon: '🎬', name: 'Film & Music',    desc: 'Discuss movies, TV shows, and music as a native-speaking friend would.' },
  { id: 'immersion-debate', icon: '🗣️', name: 'Opinion & Debate', desc: 'Share views and argue positions in real native-level discourse.' },
];

function renderImmersionActivities(container) {
  const lang = languages.find(l => l.code === selectedLang) || extraLanguages.find(l => l.code === selectedLang);
  const langName = lang ? lang.name : 'your target language';

  container.innerHTML = `
    <div class="immersion-warning">
      <div class="immersion-warning-icon">🔵</div>
      <div>
        <strong>Full ${langName} Immersion</strong>
        <p>The AI will speak only in ${langName} — no English, no translations, no corrections. Choose a scene and dive in.</p>
        <ul class="immersion-rules-list">
          <li>Zero English from the AI, no matter what</li>
          <li>If you write in English, the AI continues as if you responded in ${langName}</li>
          <li>Grammar errors are ignored unless meaning breaks down</li>
        </ul>
      </div>
    </div>
    <div class="immersion-grid">
      ${IMMERSION_SCENARIOS.map(s => `
        <div
          class="immersion-card"
          id="topic-${s.id}"
          onclick="selectTopic('${s.id}')"
          role="radio"
          aria-checked="false"
          tabindex="0"
          onkeydown="if(event.key==='Enter'||event.key===' ')selectTopic('${s.id}')"
        >
          <div class="immersion-card-icon">${s.icon}</div>
          <div class="immersion-card-name">${s.name}</div>
          <div class="immersion-card-desc">${s.desc}</div>
        </div>
      `).join('')}
    </div>
  `;
}

const CULTURAL_ACTIVITIES = [
  {
    id: 'cultural-context',
    icon: '🏛️',
    name: 'Cultural Context Lessons',
    desc: 'Learn social norms, etiquette, and unwritten rules through guided cultural discussion.',
    tag: 'Culture',
  },
  {
    id: 'cultural-stories',
    icon: '📖',
    name: 'Story-Based Learning',
    desc: 'Immerse yourself in short authentic stories set in real cultural contexts.',
    tag: 'Stories',
  },
  {
    id: 'cultural-idioms',
    icon: '💬',
    name: 'Idioms & Expressions',
    desc: 'Master common idioms, proverbs, and sayings with their cultural origins and real usage.',
    tag: 'Idioms',
  },
  {
    id: 'cultural-food',
    icon: '🍜',
    name: 'Food & Cuisine Culture',
    desc: 'Explore food traditions, dining customs, regional dishes, and culinary vocabulary.',
    tag: 'Food',
  },
  {
    id: 'cultural-history',
    icon: '🎭',
    name: 'History & Traditions',
    desc: 'Discover festivals, historical milestones, and the stories behind regional cultural traditions.',
    tag: 'History',
  },
];

function renderCulturalActivities(container) {
  container.innerHTML = `
    <p class="activity-section-intro">Choose a cultural topic. Each session blends language practice with genuine cultural insight.</p>
    <div class="grammar-grid">
      ${CULTURAL_ACTIVITIES.map(a => `
        <div
          class="grammar-card"
          id="topic-${a.id}"
          onclick="selectTopic('${a.id}')"
          role="radio"
          aria-checked="false"
          tabindex="0"
          onkeydown="if(event.key==='Enter'||event.key===' ')selectTopic('${a.id}')"
        >
          <div class="grammar-card-header">
            <div class="grammar-card-icon">${a.icon}</div>
            <div class="cultural-tag">${a.tag}</div>
          </div>
          <div class="grammar-card-name">${a.name}</div>
          <div class="grammar-card-desc">${a.desc}</div>
        </div>
      `).join('')}
    </div>
  `;
}

function renderConversationalActivities(container) {
  const regularCats = new Set(['Everyday Life', 'Social', 'Travel & Leisure', 'Health & Learning', 'Professional']);
  const regularTopics = topics.filter(t => regularCats.has(t.category));
  const rolePlayTopics = topics.filter(t => t.category === 'Role-Play Scenarios');
  const travelTopics = topics.filter(t => t.category === 'AI Travel Mode');

  container.innerHTML = `
    <div class="activity-tabs">
      <button class="activity-tab-btn active" onclick="switchActivityTab(event, 'tab-topics')">Topics</button>
      <button class="activity-tab-btn" onclick="switchActivityTab(event, 'tab-roleplay')">Role-Play Scenarios</button>
      <button class="activity-tab-btn" onclick="switchActivityTab(event, 'tab-travel')">AI Travel Mode</button>
    </div>

    <div class="activity-tab-panel active" id="tab-topics">
      ${renderTopicsByCategory(regularTopics)}
    </div>

    <div class="activity-tab-panel" id="tab-roleplay">
      <div class="topic-grid">
        ${rolePlayTopics.map(t => topicCardHTML(t)).join('')}
      </div>
    </div>

    <div class="activity-tab-panel" id="tab-travel">
      <div class="topic-grid travel-grid">
        ${travelTopics.map(t => travelCardHTML(t)).join('')}
      </div>
    </div>
  `;
}

function renderTopicsByCategory(topicList) {
  const categories = {};
  topicList.forEach(t => {
    if (!categories[t.category]) categories[t.category] = [];
    categories[t.category].push(t);
  });

  return Object.entries(categories).map(([cat, items]) => `
    <div class="category-section">
      <div class="category-title">${cat}</div>
      <div class="topic-grid">
        ${items.map(t => topicCardHTML(t)).join('')}
      </div>
    </div>
  `).join('');
}

function topicCardHTML(t) {
  return `
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
    </div>`;
}

function travelCardHTML(t) {
  return `
    <div
      class="travel-card"
      id="topic-${t.id}"
      onclick="selectTopic('${t.id}')"
      role="radio"
      aria-checked="false"
      tabindex="0"
      onkeydown="if(event.key==='Enter'||event.key===' ')selectTopic('${t.id}')"
    >
      <div class="travel-card-flag">${t.icon}</div>
      <div class="travel-card-city">${t.name}</div>
      <div class="travel-card-desc">${t.description}</div>
    </div>`;
}

function renderComingSoonPanel(container, modeInfo) {
  const descriptions = {
    grammar:   'Vocabulary Builder, Sentence Construction, Pronunciation Training, Listening Exercises, and Writing Coach — all coming soon.',
    cultural:  'Cultural Context Lessons and Story-Based Learning to understand the culture behind the language — coming soon.',
    immersion: 'Full interface immersion — every button, label, and prompt switches to your target language — coming soon.',
  };

  container.innerHTML = `
    <div class="coming-soon-panel">
      <div class="coming-soon-panel-icon">${modeInfo.icon}</div>
      <h3>${modeInfo.name}</h3>
      <p>${descriptions[modeInfo.id] || 'This mode is coming soon!'}</p>
    </div>
  `;
}

function switchActivityTab(event, tabId) {
  document.querySelectorAll('.activity-tab-btn').forEach(btn => btn.classList.remove('active'));
  event.target.classList.add('active');

  document.querySelectorAll('.activity-tab-panel').forEach(panel => panel.classList.remove('active'));
  document.getElementById(tabId).classList.add('active');

  if (selectedTopic) {
    const prev = document.getElementById(`topic-${selectedTopic}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }
    selectedTopic = null;
    updateStartBar();
  }
}

/* ── Topic selection ────────────────────────────────────────────────────────── */
function selectTopic(id) {
  if (selectedTopic) {
    const prev = document.getElementById(`topic-${selectedTopic}`);
    if (prev) { prev.classList.remove('selected'); prev.setAttribute('aria-checked', 'false'); }
  }
  selectedTopic = id;
  const card = document.getElementById(`topic-${id}`);
  if (card) { card.classList.add('selected'); card.setAttribute('aria-checked', 'true'); }
  card?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  updateStartBar();
}

/* ── Start bar ──────────────────────────────────────────────────────────────── */
function updateStartBar() {
  const bar  = document.getElementById('startBar');
  const info = document.getElementById('selectionInfo');

  const conversationalModes = new Set(['conversational', 'grammar', 'cultural', 'immersion']);
  if (!selectedLang || !selectedLevel || !conversationalModes.has(selectedMode) || !selectedTopic) {
    bar.classList.remove('visible');
    return;
  }

  const lang  = languages.find(l => l.code === selectedLang) || extraLanguages.find(l => l.code === selectedLang);
  const lv    = levels.find(l => l.id === selectedLevel);
  const topic = topics.find(t => t.id === selectedTopic)
    || GRAMMAR_ACTIVITIES.find(a => a.id === selectedTopic)
    || CULTURAL_ACTIVITIES.find(a => a.id === selectedTopic)
    || IMMERSION_SCENARIOS.find(s => s.id === selectedTopic);
  const pers  = selectedPersonality
    ? personalities.find(p => p.id === selectedPersonality)
    : null;

  const startBtn = document.getElementById('startBtn');
  if (startBtn) {
    const btnLabels = { grammar: 'Start Exercise →', cultural: 'Start Session →', immersion: 'Enter Immersion →' };
    startBtn.textContent = btnLabels[selectedMode] || 'Start Conversation →';
  }

  info.innerHTML = `
    <span class="badge badge-purple">${lang.flag} ${lang.name}</span>
    <span style="color:var(--text-3)">·</span>
    <span class="badge badge-green">${lv.emoji} Level ${lv.id} — ${lv.label}</span>
    ${pers && selectedMode === 'conversational' ? `<span style="color:var(--text-3)">·</span><span class="badge badge-yellow">${pers.icon} ${pers.name}</span>` : ''}
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
      language:    selectedLang,
      level:       selectedLevel,
      topic:       selectedTopic,
      personality: selectedPersonality || '',
    });

    if (!data) return;

    const params = new URLSearchParams({
      session:     data.session_id,
      language:    data.language,
      level:       selectedLevel,
      topic:       data.topic,
      topicName:   data.topic_name,
      personality: data.personality || '',
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
renderModes();
loadData();
