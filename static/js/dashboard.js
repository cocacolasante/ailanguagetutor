requireAuth();

/* ── User state ─────────────────────────────────────────────────────────────── */
let currentUser   = null;
let selectedMode  = null;
let selectedTopic = null;

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(async function init() {
  try {
    currentUser = await API.get('/api/auth/me');
  } catch {
    window.location.href = '/';
    return;
  }

  // Navbar
  const u = currentUser;
  const navAvatar = document.getElementById('navAvatar');
  if (navAvatar) navAvatar.textContent = (u.username || '?')[0].toUpperCase();
  const navUsername = document.getElementById('navUsername');
  if (navUsername) navUsername.textContent = u.username || '';
  if (u.is_admin) document.getElementById('adminLink')?.classList.remove('hidden');

  // Banners
  if (!u.email_verified) document.getElementById('emailVerifyBanner')?.classList.remove('hidden');
  if (u.subscription_status === 'trialing') {
    const tb = document.getElementById('trialBanner');
    tb?.classList.remove('hidden');
    if (u.trial_ends_at) {
      const days = Math.max(0, Math.ceil((new Date(u.trial_ends_at) - Date.now()) / 86400000));
      const el = document.getElementById('trialBannerText');
      if (el) el.textContent = `⏰ ${days} day${days !== 1 ? 's' : ''} left on trial`;
    }
  }
  if (u.subscription_status === 'past_due') document.getElementById('pastdueBanner')?.classList.remove('hidden');

  // Prefs banner
  if (!u.pref_language || !u.pref_level) {
    document.getElementById('prefsBanner')?.classList.remove('hidden');
  }

  // Greeting
  const hour = new Date().getHours();
  const timeOfDay = hour < 12 ? 'morning' : hour < 18 ? 'afternoon' : 'evening';
  document.getElementById('greeting-time').textContent = timeOfDay;
  document.getElementById('greeting-name').textContent = u.username || 'learner';

  // Update subtitle based on pref state
  if (u.pref_language) {
    const langName = LANG_META[u.pref_language]?.name || u.pref_language;
    document.getElementById('greetingSubtitle').textContent =
      `Learning ${langName} · Level ${u.pref_level || '?'} · ${u.pref_personality ? PERSONALITY_NAMES[u.pref_personality] || 'Custom tutor' : 'No tutor set'}`;
  }

  // Stats
  loadStats();

  // Build the new lesson mode grid (pre-render, hidden until needed)
  renderModeGrid();
})();

/* ── Language / personality metadata ────────────────────────────────────────── */
const LANG_META = {
  it: { flag: '🇮🇹', name: 'Italian' },
  es: { flag: '🇪🇸', name: 'Spanish' },
  pt: { flag: '🇧🇷', name: 'Portuguese' },
  // fr: { flag: '🇫🇷', name: 'French' },
  // de: { flag: '🇩🇪', name: 'German' },
  // ja: { flag: '🇯🇵', name: 'Japanese' },
  // ru: { flag: '🇷🇺', name: 'Russian' },
  // ro: { flag: '🇷🇴', name: 'Romanian' },
  // zh: { flag: '🇨🇳', name: 'Chinese' },
};

const PERSONALITY_NAMES = {
  '':                   'No Preference',
  'professor':          'The Professor',
  'friendly-partner':   'Friendly Partner',
  'bartender':          'The Bartender',
  'business-executive': 'Business Executive',
  'travel-guide':       'Travel Guide',
};

const LEVEL_NAMES = {
  1: 'Beginner', 2: 'Elementary', 3: 'Intermediate', 4: 'Upper-Intermediate', 5: 'Fluent',
};

/* ── Stats ──────────────────────────────────────────────────────────────────── */
async function loadStats() {
  try {
    const stats = await API.get('/api/user/stats');
    if (!stats) return;
    document.getElementById('streakValue').textContent = stats.streak ?? 0;
    document.getElementById('fpValue').textContent     = stats.total_fp ?? 0;
    document.getElementById('convCount').textContent   = stats.conversation_count ?? 0;

    const badges = stats.achievements || [];
    if (badges.length) {
      const row  = document.getElementById('achievementsRow');
      const list = document.getElementById('achievementsList');
      row?.classList.remove('hidden');
      const last3 = badges.slice(-3).reverse();
      list.innerHTML = last3.map(id => {
        const b = BADGE_META[id] || { icon: '🏅', name: id };
        return `<div class="achievement-pill" title="${b.name}">${b.icon} ${b.name}</div>`;
      }).join('');
    }
  } catch {}
}

const BADGE_META = {
  streak_3:      { icon: '🔥', name: '3-Day Streak' },
  streak_7:      { icon: '🔥', name: 'Week Warrior' },
  streak_30:     { icon: '🌟', name: 'Monthly Master' },
  streak_100:    { icon: '💎', name: 'Century Streak' },
  first_conv:    { icon: '👶', name: 'First Steps' },
  conv_10:       { icon: '📖', name: 'Getting Started' },
  conv_50:       { icon: '🎓', name: 'Dedicated Learner' },
  conv_100:      { icon: '🏆', name: 'Language Champion' },
  fp_100:        { icon: '⭐', name: 'FP Collector' },
  fp_500:        { icon: '🌟', name: 'FP Enthusiast' },
  fp_1000:       { icon: '💫', name: 'FP Expert' },
  fp_5000:       { icon: '✨', name: 'FP Legend' },
  lang_level_5:  { icon: '📈', name: 'Intermediate' },
  lang_level_10: { icon: '🎯', name: 'Advanced' },
  lang_level_20: { icon: '👑', name: 'Master' },
};

/* ── Continue Lesson panel ──────────────────────────────────────────────────── */
function showContinuePanel() {
  document.getElementById('continuePanel').classList.remove('hidden');
  document.getElementById('newLessonPanel').classList.add('hidden');
  document.getElementById('startBar').classList.remove('show');
  selectedTopic = null;
  selectedMode  = null;
  loadRecentLessons();
}

async function loadRecentLessons() {
  const grid = document.getElementById('recentLessonsList');
  grid.innerHTML = '<div class="conv-loading-state" style="padding:32px 0"><div class="spinner"></div><p>Loading…</p></div>';
  try {
    const data = await API.get('/api/conversation/records');
    const records = data?.records || [];
    if (!records.length) {
      grid.innerHTML = '<p style="color:var(--text-2);padding:24px 0">No recent lessons yet. Start your first one!</p>';
      return;
    }
    const recent = records.slice(0, 3);
    grid.innerHTML = recent.map(r => {
      const lang = LANG_META[r.language] || { flag: '🌐', name: r.language };
      const date = new Date(r.created_at || r.ended_at || Date.now()).toLocaleDateString([], { month: 'short', day: 'numeric' });
      const personality = r.personality ? (PERSONALITY_NAMES[r.personality] || r.personality) : 'Default tutor';
      return `
        <div class="recent-lesson-card" onclick="continueLesson(${JSON.stringify(r).replace(/"/g, '&quot;')})">
          <div class="recent-lesson-flag">${lang.flag}</div>
          <div class="recent-lesson-body">
            <div class="recent-lesson-topic">${r.topic_name || r.topic}</div>
            <div class="recent-lesson-meta">${lang.name} · Level ${r.level} · ${personality}</div>
            <div class="recent-lesson-date">${date}</div>
          </div>
          <div class="recent-lesson-arrow">→</div>
        </div>`;
    }).join('');
  } catch {
    grid.innerHTML = '<p style="color:var(--text-2);padding:24px 0">Could not load recent lessons.</p>';
  }
}

function continueLesson(record) {
  // Start a new session with the same parameters as the selected record
  startConversationWithParams(record.language, record.level, record.personality || '', record.topic, record.topic_name || record.topic);
}

/* ── New Lesson panel ───────────────────────────────────────────────────────── */
function showNewLessonPanel() {
  document.getElementById('newLessonPanel').classList.remove('hidden');
  document.getElementById('continuePanel').classList.add('hidden');
  document.getElementById('startBar').classList.remove('show');
  selectedTopic = null;
  selectedMode  = null;
  // Reset activity section
  document.getElementById('activitySection')?.classList.add('hidden');
  // Deselect mode cards
  document.querySelectorAll('.mode-card').forEach(c => c.classList.remove('selected'));
}

/* ── Learning mode grid ─────────────────────────────────────────────────────── */
const LEARNING_MODES = [
  {
    id: 'conversational',
    icon: '💬',
    name: 'Conversational Practice',
    desc: 'Real conversations, role-play scenarios, and travel simulations.',
    available: true,
  },
  {
    id: 'grammar',
    icon: '📝',
    name: 'Grammar & Skills',
    desc: 'Vocabulary builder, sentence construction, pronunciation, listening, writing.',
    available: true,
  },
  {
    id: 'cultural',
    icon: '🌍',
    name: 'Cultural Learning',
    desc: 'Cultural context lessons and story-based learning.',
    available: false,
  },
  {
    id: 'immersion',
    icon: '🔵',
    name: 'Immersion Mode',
    desc: 'The entire session is conducted only in your target language.',
    available: false,
  },
];

function renderModeGrid() {
  const grid = document.getElementById('learningModeGrid');
  if (!grid) return;
  grid.innerHTML = LEARNING_MODES.map(m => `
    <div class="mode-card${m.available ? '' : ' mode-card-coming-soon'}" id="mode-${m.id}"
         onclick="${m.available ? `selectMode('${m.id}')` : `showComingSoon('${m.name}')`}">
      <div class="mode-card-icon">${m.icon}</div>
      <div class="mode-card-body">
        <div class="mode-card-name">${m.name}${m.available ? '' : ' <span class="coming-soon-badge">Coming Soon</span>'}</div>
        <div class="mode-card-desc">${m.desc}</div>
      </div>
    </div>`).join('');
}

function showComingSoon(name) {
  const container = document.getElementById('activityContainer');
  const section   = document.getElementById('activitySection');
  container.innerHTML = `
    <div class="coming-soon-panel">
      <div class="coming-soon-panel-icon">🚧</div>
      <h3>${name}</h3>
      <p>This learning mode is coming soon. Stay tuned for updates!</p>
    </div>`;
  section.classList.remove('hidden');
}

function selectMode(modeId) {
  selectedMode  = modeId;
  selectedTopic = null;
  document.getElementById('startBar').classList.remove('show');

  // Highlight selected mode card
  document.querySelectorAll('.mode-card').forEach(c => c.classList.remove('selected'));
  document.getElementById(`mode-${modeId}`)?.classList.add('selected');

  // Show activity section
  const section = document.getElementById('activitySection');
  section.classList.remove('hidden');
  section.scrollIntoView({ behavior: 'smooth', block: 'start' });

  renderActivitySection(modeId);
}

/* ── Activity / topic data ──────────────────────────────────────────────────── */

const CONVERSATIONAL_TOPICS = [
  {
    category: 'Everyday Life',
    topics: [
      { id: 'general',      icon: '💬', name: 'General Conversation',  desc: 'Free-form practice on any subject' },
      { id: 'daily-recap',  icon: '📅', name: 'Daily Recap',           desc: 'Talk about your day and recent events' },
      { id: 'future-plans', icon: '🗓️', name: 'Future Plans',         desc: 'Discuss goals, dreams, and upcoming events' },
      { id: 'home',         icon: '🏠', name: 'Home & Living',         desc: 'Describe your home, neighbourhood, and daily routine' },
      { id: 'food-dining',  icon: '🍽️', name: 'Food & Dining',        desc: 'Restaurants, recipes, and food culture' },
      { id: 'shopping',     icon: '🛍️', name: 'Shopping',             desc: 'Stores, fashion, and buying decisions' },
      { id: 'family',       icon: '👨‍👩‍👧', name: 'Family Life',       desc: 'Relationships, traditions, and home life' },
    ],
  },
  {
    category: 'Social & Entertainment',
    topics: [
      { id: 'culture',        icon: '🎭', name: 'Arts & Culture',       desc: 'Music, art, movies, and traditions' },
      { id: 'sports',         icon: '⚽', name: 'Sports & Fitness',     desc: 'Teams, workouts, and sporting events' },
      { id: 'entertainment',  icon: '🎬', name: 'Entertainment',        desc: 'TV, film, gaming, and pop culture' },
      { id: 'news',           icon: '📰', name: 'News & Current Events', desc: 'Headlines, global issues, and opinions' },
    ],
  },
  {
    category: 'Travel & Nature',
    topics: [
      { id: 'travel',      icon: '✈️', name: 'Travel',       desc: 'Trips, destinations, and travel stories' },
      { id: 'environment', icon: '🌿', name: 'Environment',  desc: 'Climate, sustainability, and nature' },
    ],
  },
  {
    category: 'Health & Learning',
    topics: [
      { id: 'health',    icon: '🏥', name: 'Health & Wellness', desc: 'Fitness, medical care, and mental health' },
      { id: 'education', icon: '📚', name: 'Education',         desc: 'School, learning strategies, and academia' },
    ],
  },
  {
    category: 'Professional',
    topics: [
      { id: 'work',       icon: '💼', name: 'Work & Career',    desc: 'Jobs, interviews, and professional life' },
      { id: 'technology', icon: '💻', name: 'Technology',       desc: 'Gadgets, software, and the digital world' },
      { id: 'cloud',      icon: '☁️', name: 'Cloud & SaaS',     desc: 'Cloud tools, infrastructure, and tech teams' },
      { id: 'marketing',  icon: '📊', name: 'Marketing',        desc: 'Branding, campaigns, and digital marketing' },
      { id: 'finance',    icon: '💰', name: 'Finance',          desc: 'Money, investing, and financial planning' },
    ],
  },
  {
    category: 'Role-Play Scenarios',
    topics: [
      { id: 'role-restaurant',    icon: '🍽️', name: 'Restaurant Ordering',   desc: 'Order food, ask about the menu, and pay the bill' },
      { id: 'role-job-interview', icon: '👔', name: 'Job Interview',          desc: 'Practice professional interviews and workplace language' },
      { id: 'role-airport',       icon: '✈️', name: 'Airport & Travel',       desc: 'Check in, security, boarding, and asking for help' },
      { id: 'role-doctor',        icon: '🏥', name: 'Doctor Visit',           desc: 'Describe symptoms, understand medical advice' },
      { id: 'role-business',      icon: '💼', name: 'Business Meeting',       desc: 'Negotiate, present ideas, and follow business etiquette' },
      { id: 'role-apartment',     icon: '🏠', name: 'Renting an Apartment',   desc: 'View apartments, negotiate rent, sign agreements' },
      { id: 'role-directions',    icon: '🗺️', name: 'Asking Directions',     desc: 'Navigate a city, understand landmarks and transit' },
    ],
  },
  {
    category: 'AI Travel Mode',
    topics: [
      { id: 'travel-rome',      icon: '🇮🇹', name: 'Rome, Italy',        desc: 'Explore Rome: food, art, navigation, and local culture' },
      { id: 'travel-barcelona', icon: '🇪🇸', name: 'Barcelona, Spain',   desc: "Navigate Barcelona's tapas bars, beaches, and architecture" },
      { id: 'travel-paris',     icon: '🇫🇷', name: 'Paris, France',      desc: 'Paris café culture, museums, and everyday Parisian life' },
      { id: 'travel-tokyo',     icon: '🇯🇵', name: 'Tokyo, Japan',       desc: "Tokyo's subway, restaurants, and cultural etiquette" },
      { id: 'travel-lisbon',    icon: '🇵🇹', name: 'Lisbon, Portugal',   desc: "Lisbon's neighborhoods, trams, and traditional cuisine" },
    ],
  },
];

const VOCAB_TOPICS = [
  { id: 'general',      icon: '💬', name: 'General',          desc: 'Everyday words and common expressions' },
  { id: 'food-dining',  icon: '🍽️', name: 'Food & Dining',   desc: 'Restaurant, recipes, and cuisine words' },
  { id: 'shopping',     icon: '🛍️', name: 'Shopping',        desc: 'Clothing, stores, and prices' },
  { id: 'family',       icon: '👨‍👩‍👧', name: 'Family Life',  desc: 'Relationships and household words' },
  { id: 'travel',       icon: '✈️', name: 'Travel',           desc: 'Airports, hotels, and directions' },
  { id: 'health',       icon: '🏥', name: 'Health',           desc: 'Body, symptoms, and medical terms' },
  { id: 'work',         icon: '💼', name: 'Work & Career',    desc: 'Workplace, jobs, and professional life' },
  { id: 'technology',   icon: '💻', name: 'Technology',       desc: 'Gadgets, software, and digital terms' },
];

const GRAMMAR_SKILLS = [
  { id: 'vocab',  icon: '📚', name: 'Vocabulary Builder', desc: 'Flashcard mode — learn, hear, and speak new words', available: true },
  { id: 'sentences', icon: '✏️', name: 'Sentence Construction', desc: 'Build grammatically correct sentences', available: false },
  { id: 'pronunciation', icon: '🗣️', name: 'Pronunciation Practice', desc: 'Perfect your pronunciation with drills', available: false },
  { id: 'listening', icon: '👂', name: 'Listening Comprehension', desc: 'Improve listening through short passages', available: false },
  { id: 'writing', icon: '📝', name: 'Writing Coach', desc: 'Submit writing for grammar corrections', available: false },
];

// Tracks which grammar sub-step we're in: null | 'skills' | 'vocab-topics'
let grammarSubStep = null;

function renderActivitySection(modeId) {
  const container = document.getElementById('activityContainer');
  if (modeId === 'grammar') {
    grammarSubStep = 'skills';
    renderGrammarSkillsPicker(container);
    return;
  }
  if (modeId !== 'conversational') {
    // Coming soon panel already handled in showComingSoon
    return;
  }
  container.innerHTML = CONVERSATIONAL_TOPICS.map(cat => `
    <div class="topic-category">
      <div class="topic-category-label">${cat.category}</div>
      <div class="topic-grid">
        ${cat.topics.map(t => `
          <div class="topic-card" id="topic-${t.id}" onclick="selectTopic('${t.id}', '${escapeAttr(t.name)}')">
            <span class="topic-icon">${t.icon}</span>
            <div class="topic-info">
              <div class="topic-name">${t.name}</div>
              <div class="topic-desc">${t.desc}</div>
            </div>
          </div>`).join('')}
      </div>
    </div>`).join('');
}

function renderGrammarSkillsPicker(container) {
  container.innerHTML = `
    <div class="topic-category">
      <div class="topic-category-label">Choose a Skill</div>
      <div class="topic-grid">
        ${GRAMMAR_SKILLS.map(s => `
          <div class="topic-card${s.available ? '' : ' mode-card-coming-soon'}"
               onclick="${s.available ? `selectGrammarSkill('${s.id}')` : `showComingSoon('${escapeAttr(s.name)}')`}">
            <span class="topic-icon">${s.icon}</span>
            <div class="topic-info">
              <div class="topic-name">${s.name}${s.available ? '' : ' <span class="coming-soon-badge">Soon</span>'}</div>
              <div class="topic-desc">${s.desc}</div>
            </div>
          </div>`).join('')}
      </div>
    </div>`;
}

function selectGrammarSkill(skillId) {
  if (skillId === 'vocab') {
    grammarSubStep = 'vocab-topics';
    const container = document.getElementById('activityContainer');
    container.innerHTML = `
      <div class="topic-category">
        <div class="topic-category-label">Choose a Topic</div>
        <div class="topic-grid">
          ${VOCAB_TOPICS.map(t => `
            <div class="topic-card" id="vocab-topic-${t.id}"
                 onclick="selectVocabTopic('${t.id}', '${escapeAttr(t.name)}')">
              <span class="topic-icon">${t.icon}</span>
              <div class="topic-info">
                <div class="topic-name">${t.name}</div>
                <div class="topic-desc">${t.desc}</div>
              </div>
            </div>`).join('')}
        </div>
      </div>`;
  }
}

function selectVocabTopic(id, name) {
  const u = currentUser;
  if (!u?.pref_language || !u?.pref_level) {
    alert('Please set your language and level in your profile first.');
    return;
  }
  const vocabParams = new URLSearchParams({
    language:  u.pref_language,
    level:     u.pref_level,
    topic:     id,
    topicName: name,
  });
  window.location.href = '/vocab.html?' + vocabParams.toString();
}

function selectTopic(id, name) {
  selectedTopic = { id, name };
  document.querySelectorAll('.topic-card').forEach(c => c.classList.remove('selected'));
  document.getElementById(`topic-${id}`)?.classList.add('selected');
  updateStartBar();
}

/* ── Start bar ──────────────────────────────────────────────────────────────── */
function updateStartBar() {
  if (!selectedTopic || !currentUser?.pref_language) {
    document.getElementById('startBar').classList.remove('show');
    return;
  }
  const u    = currentUser;
  const lang = LANG_META[u.pref_language] || { flag: '🌐', name: u.pref_language };
  const lvl  = LEVEL_NAMES[u.pref_level]  || `Level ${u.pref_level}`;
  const pers = u.pref_personality ? (PERSONALITY_NAMES[u.pref_personality] || u.pref_personality) : '';

  const infoEl = document.getElementById('selectionInfo');
  infoEl.innerHTML = `
    <span class="start-chip">${lang.flag} ${lang.name}</span>
    <span class="start-chip">Level ${u.pref_level} · ${lvl}</span>
    ${pers ? `<span class="start-chip">🎭 ${pers}</span>` : ''}
    <span class="start-chip">📌 ${selectedTopic.name}</span>
  `;
  document.getElementById('startBar').classList.add('show');
}

/* ── Start conversation ─────────────────────────────────────────────────────── */
async function startConversation() {
  if (!selectedTopic || !currentUser?.pref_language) return;
  const u = currentUser;
  startConversationWithParams(u.pref_language, u.pref_level || 3, u.pref_personality || '', selectedTopic.id, selectedTopic.name);
}

async function startConversationWithParams(language, level, personality, topicId, topicName) {
  const btn = document.getElementById('startBtn');
  if (btn) { btn.disabled = true; btn.textContent = 'Starting…'; }
  try {
    const session = await API.post('/api/conversation/start', { language, level, personality, topic: topicId });
    const params  = new URLSearchParams({
      session:     session.session_id,
      language:    session.language,
      level:       session.level,
      topic:       session.topic,
      topicName:   topicName || session.topic,
      personality: session.personality || '',
    });
    window.location.href = '/conversation.html?' + params.toString();
  } catch (err) {
    if (btn) { btn.disabled = false; btn.textContent = 'Start Conversation →'; }
    alert('Could not start conversation: ' + (err.message || 'Unknown error'));
  }
}

/* ── Auth logout ────────────────────────────────────────────────────────────── */
async function logout() {
  try { await API.post('/api/auth/logout', {}); } catch {}
  localStorage.removeItem('auth_token');
  window.location.href = '/';
}

/* ── Helpers ────────────────────────────────────────────────────────────────── */
function escapeAttr(str) {
  return String(str).replace(/'/g, "\\'").replace(/"/g, '&quot;');
}
