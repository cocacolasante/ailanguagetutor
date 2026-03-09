requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const language  = params.get('language') || 'it';
const level     = parseInt(params.get('level') || '1', 10);
const topic     = params.get('topic')     || 'general';
const topicName = params.get('topicName') || 'General';

/* ── State ──────────────────────────────────────────────────────────────────── */
let sentences   = [];    // Sentence[]
let currentIdx  = 0;
let attempts    = 0;     // 1-2 max per sentence
let results     = [];    // {sentence_id, grammar_tip, correct}[]
let isListening = false;
let recognition = null;
let hasSpeechAPI = false;
let currentCorrectSentence = '';

const MAX_ATTEMPTS = 2;

/* ── Language maps ───────────────────────────────────────────────────────────── */
const LANG_BCP47 = { it: 'it-IT', es: 'es-ES', pt: 'pt-BR' };
const LANG_NAMES = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };
const LANG_FLAGS = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' };

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(async function init() {
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (SpeechRecognition) {
    hasSpeechAPI = true;
    recognition = new SpeechRecognition();
    recognition.continuous     = false;
    recognition.interimResults = false;
    recognition.lang           = LANG_BCP47[language] || 'it-IT';
    recognition.onresult = (e) => {
      const transcript = e.results[0][0].transcript;
      stopListening();
      document.getElementById('answerInput').value = transcript;
      submitAnswer();
    };
    recognition.onerror = () => stopListening();
    recognition.onend   = () => { if (isListening) stopListening(); };
  } else {
    document.getElementById('speakBtn').classList.add('hidden');
  }

  await loadSession();
})();

/* ── Load session ───────────────────────────────────────────────────────────── */
async function loadSession() {
  try {
    const data = await API.post('/api/sentences/session', { language, level, topic });
    sentences = data.sentences || [];
    if (!sentences.length) {
      showError('No sentences returned. Please try again.');
      return;
    }

    const langFlag = LANG_FLAGS[language] || '🌐';
    const langName = LANG_NAMES[language] || language;
    document.getElementById('headerTitle').textContent = `${topicName} — Sentence Builder`;
    document.getElementById('headerSub').textContent   =
      `${langFlag} ${langName} · Level ${level} · ${sentences.length} sentences`;
    document.getElementById('translateLabel').textContent =
      `Translate into ${langName}`;

    document.getElementById('loadingState').classList.add('hidden');
    document.getElementById('sentenceContainer').classList.remove('hidden');

    renderSentence(sentences[0]);
  } catch (err) {
    showError('Failed to load sentences. ' + (err.message || ''));
  }
}

/* ── Render sentence ─────────────────────────────────────────────────────────── */
function renderSentence(s) {
  document.getElementById('sentenceEnglish').textContent = s.english;
  document.getElementById('answerInput').value = '';
  document.getElementById('answerInput').disabled = false;
  document.getElementById('submitBtn').disabled = false;
  document.getElementById('feedbackZone').classList.add('hidden');
  document.getElementById('feedbackStatus').textContent = '';
  document.getElementById('feedbackText').textContent = '';
  document.getElementById('correctedForm').classList.add('hidden');
  document.getElementById('retryBtn').classList.add('hidden');
  document.getElementById('nextBtn').classList.add('hidden');
  document.getElementById('playCorrectBtn').classList.add('hidden');
  attempts = 0;
  updateProgress();
  document.getElementById('answerInput').focus();
}

/* ── Progress ───────────────────────────────────────────────────────────────── */
function updateProgress() {
  const total   = sentences.length;
  const current = currentIdx + 1;
  const pct     = (currentIdx / total) * 100;
  document.getElementById('progressFill').style.width  = pct + '%';
  document.getElementById('progressLabel').textContent = `Sentence ${current} of ${total}`;
}

/* ── Submit answer ───────────────────────────────────────────────────────────── */
async function submitAnswer() {
  const s = sentences[currentIdx];
  const userAnswer = document.getElementById('answerInput').value.trim();
  if (!userAnswer) return;

  attempts++;
  document.getElementById('submitBtn').disabled = true;
  document.getElementById('answerInput').disabled = true;

  try {
    const result = await API.post('/api/sentences/check', {
      english:         s.english,
      target_expected: s.target,
      user_answer:     userAnswer,
      language,
    });
    showFeedback(result, s);
  } catch {
    // Network error — treat as incorrect attempt
    showFeedback({ correct: false, feedback: 'Could not check — please try again.', corrected: '' }, s);
  }
}

/* ── Show feedback ───────────────────────────────────────────────────────────── */
function showFeedback(result, s) {
  const zone          = document.getElementById('feedbackZone');
  const statusEl      = document.getElementById('feedbackStatus');
  const textEl        = document.getElementById('feedbackText');
  const corrEl        = document.getElementById('correctedForm');
  const retryBtn      = document.getElementById('retryBtn');
  const nextBtn       = document.getElementById('nextBtn');
  const playCorrectBtn = document.getElementById('playCorrectBtn');

  zone.classList.remove('hidden');
  corrEl.classList.add('hidden');
  retryBtn.classList.add('hidden');
  nextBtn.classList.add('hidden');
  playCorrectBtn.classList.add('hidden');
  currentCorrectSentence = '';

  if (result.correct) {
    statusEl.textContent = '✓ Correct!';
    statusEl.className   = 'sentence-feedback-status correct';
    textEl.textContent   = result.feedback || '';
    results.push({ sentence_id: s.id, grammar_tip: s.grammar_tip, correct: true });
    // Play the correct sentence audio then auto-advance
    const target = result.corrected || s.target || '';
    if (target) {
      currentCorrectSentence = target;
      playCorrectBtn.classList.remove('hidden');
      playCorrectSentence();
    }
    nextBtn.classList.remove('hidden');
    setTimeout(() => nextSentence(), 2000);
  } else if (attempts < MAX_ATTEMPTS) {
    statusEl.textContent = '✗ Not quite';
    statusEl.className   = 'sentence-feedback-status feedback';
    textEl.textContent   = result.feedback || 'Try again — check your grammar.';
    retryBtn.classList.remove('hidden');
  } else {
    // Max attempts reached
    statusEl.textContent = '✗ Incorrect';
    statusEl.className   = 'sentence-feedback-status incorrect';
    textEl.textContent   = result.feedback || '';
    const corrected = result.corrected || s.target || '';
    if (corrected) {
      corrEl.textContent = '✓ ' + corrected;
      corrEl.classList.remove('hidden');
      currentCorrectSentence = corrected;
      playCorrectBtn.classList.remove('hidden');
      playCorrectSentence();
    }
    results.push({ sentence_id: s.id, grammar_tip: s.grammar_tip, correct: false });
    nextBtn.classList.remove('hidden');
  }
}

/* ── TTS playback ────────────────────────────────────────────────────────────── */
async function playCorrectSentence() {
  if (!currentCorrectSentence) return;
  const btn = document.getElementById('playCorrectBtn');
  if (btn) { btn.disabled = true; btn.textContent = '⏳'; }
  try {
    const res = await API.binary('/api/tts', { text: currentCorrectSentence, language });
    if (!res.ok) throw new Error('TTS failed');
    const blob  = await res.blob();
    const url   = URL.createObjectURL(blob);
    const audio = new Audio(url);
    audio.onended = () => {
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Hear it'; }
    };
    audio.onerror = () => {
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Hear it'; }
    };
    await audio.play().catch(() => {
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Hear it'; }
    });
  } catch {
    if (btn) { btn.disabled = false; btn.textContent = '🔊 Hear it'; }
  }
}

/* ── Retry ───────────────────────────────────────────────────────────────────── */
function retryAnswer() {
  document.getElementById('answerInput').value = '';
  document.getElementById('answerInput').disabled = false;
  document.getElementById('submitBtn').disabled = false;
  document.getElementById('feedbackZone').classList.add('hidden');
  document.getElementById('answerInput').focus();
}

/* ── Next sentence ───────────────────────────────────────────────────────────── */
function nextSentence() {
  currentIdx++;
  if (currentIdx < sentences.length) {
    renderSentence(sentences[currentIdx]);
  } else {
    completeSession();
  }
}

/* ── Complete session ────────────────────────────────────────────────────────── */
async function completeSession() {
  document.getElementById('sentenceContainer').classList.add('hidden');

  try {
    const data = await API.post('/api/sentences/complete', {
      language,
      level,
      topic,
      topic_name: topicName,
      results,
    });

    const weakGrammar  = data.weak_grammar  || [];
    const correctCount = data.correct_count ?? results.filter(r => r.correct).length;
    const fpEarned     = data.fp_earned     ?? Math.max(10, correctCount * 8);

    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statCorrect').textContent = correctCount;
    document.getElementById('statFP').textContent      = `+${fpEarned} FP`;

    if (weakGrammar.length) {
      document.getElementById('weakSection').classList.remove('hidden');
      document.getElementById('weakList').innerHTML =
        weakGrammar.map(w => `<li>${w}</li>`).join('');
    }
  } catch {
    const correctCount = results.filter(r => r.correct).length;
    const fpEarned     = Math.max(10, correctCount * 8);
    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statCorrect').textContent = correctCount;
    document.getElementById('statFP').textContent      = `+${fpEarned} FP`;
  }

  document.getElementById('completeScreen').classList.remove('hidden');
}

/* ── Practice again ──────────────────────────────────────────────────────────── */
function practiceAgain() {
  window.location.href = window.location.href;
}

/* ── Speech recognition ──────────────────────────────────────────────────────── */
function startListening() {
  if (!hasSpeechAPI || !recognition) return;
  if (isListening) { stopListening(); return; }
  isListening = true;
  const btn = document.getElementById('speakBtn');
  if (btn) { btn.textContent = '⏹ Stop'; btn.classList.add('listening'); }
  try { recognition.start(); } catch { stopListening(); }
}

function stopListening() {
  isListening = false;
  const btn = document.getElementById('speakBtn');
  if (btn) { btn.textContent = '🎤 Speak'; btn.classList.remove('listening'); }
  try { recognition.stop(); } catch {}
}

/* ── Keyboard shortcut ───────────────────────────────────────────────────────── */
function handleTextareaKey(e) {
  // Ctrl+Enter or Cmd+Enter submits
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    submitAnswer();
  }
}

/* ── Helpers ─────────────────────────────────────────────────────────────────── */
function showError(msg) {
  document.getElementById('loadingState').innerHTML =
    `<p style="color:#f87171">${msg}</p><a href="/dashboard.html" class="btn btn-ghost btn-sm" style="margin-top:12px">← Dashboard</a>`;
}
