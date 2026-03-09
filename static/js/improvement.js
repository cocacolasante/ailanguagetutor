requireAuth();

const params   = new URLSearchParams(window.location.search);
const language = params.get('language') || '';
const level    = parseInt(params.get('level') || '1', 10);

const LANG_FLAGS = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' };
const LANG_NAMES = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };

let mistakesData = null;

(async function init() {
  if (!language) {
    window.location.href = '/dashboard.html';
    return;
  }

  try {
    mistakesData = await API.get(`/api/user/mistakes?language=${encodeURIComponent(language)}`);
  } catch (err) {
    document.getElementById('loadingState').innerHTML =
      '<p style="color:var(--text-2)">Could not load your data. <a href="/dashboard.html">Return to dashboard</a></p>';
    return;
  }

  document.getElementById('loadingState').classList.add('hidden');

  if (!mistakesData.has_mistakes) {
    document.getElementById('noMistakesState').classList.remove('hidden');
    return;
  }

  // Render improvement page
  const flag = LANG_FLAGS[language] || '🌐';
  const langName = LANG_NAMES[language] || language;
  document.getElementById('improvementSub').textContent =
    `${flag} ${langName} · Level ${level}`;

  const weakVocab   = mistakesData.weak_vocab   || [];
  const weakGrammar = mistakesData.weak_grammar || [];

  if (weakVocab.length > 0) {
    const section = document.getElementById('weakVocabSection');
    section.classList.remove('hidden');
    document.getElementById('weakVocabList').innerHTML =
      `<ul class="summary-list">${weakVocab.map(w => `<li>${escapeHtml(w)}</li>`).join('')}</ul>`;
  }

  if (weakGrammar.length > 0) {
    const section = document.getElementById('weakGrammarSection');
    section.classList.remove('hidden');
    document.getElementById('weakGrammarList').innerHTML =
      `<ul class="summary-list">${weakGrammar.map(g => `<li>${escapeHtml(g)}</li>`).join('')}</ul>`;
  }

  // Wire up buttons
  const vocabBtn = document.getElementById('btnVocabImprove');
  const sentenceBtn = document.getElementById('btnSentenceImprove');
  const convBtn = document.getElementById('btnConvImprove');

  if (weakVocab.length === 0) {
    vocabBtn.classList.add('mode-card-coming-soon');
    vocabBtn.title = 'No weak vocab recorded yet';
  } else {
    vocabBtn.addEventListener('click', startVocab);
  }

  if (weakGrammar.length === 0) {
    sentenceBtn.classList.add('mode-card-coming-soon');
    sentenceBtn.title = 'No grammar mistakes recorded yet';
  } else {
    sentenceBtn.addEventListener('click', startSentences);
  }

  convBtn.addEventListener('click', startConversation);

  document.getElementById('improvementMain').classList.remove('hidden');
})();

function startVocab() {
  const p = new URLSearchParams({
    language,
    level,
    topic:         'general',
    topicName:     'Weak Words Review',
    mistakes_mode: 'true',
  });
  window.location.href = '/vocab.html?' + p.toString();
}

function startSentences() {
  const p = new URLSearchParams({
    language,
    level,
    topic:         'general',
    topicName:     'Grammar Review',
    mistakes_mode: 'true',
  });
  window.location.href = '/sentences.html?' + p.toString();
}

async function startConversation() {
  try {
    const session = await API.post('/api/conversation/start', {
      language,
      level,
      personality: '',
      topic: 'general',
    });
    const p = new URLSearchParams({
      session:     session.session_id,
      language:    session.language,
      level:       session.level,
      topic:       session.topic,
      topicName:   'General Conversation',
      personality: session.personality || '',
    });
    window.location.href = '/conversation.html?' + p.toString();
  } catch (err) {
    alert('Could not start conversation: ' + (err.message || 'Unknown error'));
  }
}

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
