requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const language  = params.get('language') || 'it';
const level     = parseInt(params.get('level') || '1', 10);
const topic     = params.get('topic')     || 'general';
const topicName = params.get('topicName') || 'General';

/* ── State ──────────────────────────────────────────────────────────────────── */
let words      = [];     // VocabWord[]
let currentIdx = 0;
let attempts   = 0;      // 1-3 for current word
let results    = [];     // {word, correct, attempts}[]
let isFlipped  = false;
let isListening = false;
let recognition = null;
let hasSpeechAPI = false;

/* ── Language BCP-47 map ─────────────────────────────────────────────────────── */
const LANG_BCP47 = { it: 'it-IT', es: 'es-ES', pt: 'pt-BR' };
const LANG_NAMES = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(async function init() {
  // Detect SpeechRecognition support
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (SpeechRecognition) {
    hasSpeechAPI = true;
    recognition = new SpeechRecognition();
    recognition.continuous    = false;
    recognition.interimResults = false;
    recognition.lang           = LANG_BCP47[language] || 'it-IT';
    recognition.onresult = (e) => {
      const transcript = e.results[0][0].transcript;
      stopListening();
      checkPronunciation(transcript);
    };
    recognition.onerror = () => stopListening();
    recognition.onend   = () => {
      if (isListening) stopListening();
    };
  } else {
    // Show manual fallback controls
    document.getElementById('listenBtn').classList.add('hidden');
    document.getElementById('manualBtns').classList.remove('hidden');
  }

  await loadSession();
})();

/* ── Load session ───────────────────────────────────────────────────────────── */
async function loadSession() {
  try {
    const data = await API.post('/api/vocab/session', { language, level, topic });
    words = data.words || [];
    if (!words.length) {
      showError('No words returned. Please try again.');
      return;
    }

    // Update header
    const langFlag = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' }[language] || '🌐';
    document.getElementById('headerTitle').textContent = `${topicName} — Vocab Builder`;
    document.getElementById('headerSub').textContent   =
      `${langFlag} ${LANG_NAMES[language] || language} · Level ${level} · ${words.length} words`;

    // Show flashcard UI
    document.getElementById('loadingState').classList.add('hidden');
    document.getElementById('flashcardContainer').classList.remove('hidden');

    renderCard(words[0]);
    updateProgress();
    // Auto-play first word after a short delay so AudioContext can init
    setTimeout(() => playWord(), 600);
  } catch (err) {
    showError('Failed to load vocabulary. ' + (err.message || ''));
  }
}

/* ── Render card ────────────────────────────────────────────────────────────── */
function renderCard(word) {
  document.getElementById('cardWord').textContent        = word.word;
  document.getElementById('cardPhonetic').textContent    = word.phonetic ? `[ ${word.phonetic} ]` : '';
  document.getElementById('cardTranslation').textContent = word.translation;

  // Reset flip
  const card = document.getElementById('flashcard');
  card.classList.remove('flipped');
  isFlipped = false;

  // Reset attempt count display
  attempts = 0;
  clearStatus();
  updateAttemptsLabel();
}

/* ── Flip card ──────────────────────────────────────────────────────────────── */
function flipCard() {
  const card = document.getElementById('flashcard');
  isFlipped = !isFlipped;
  card.classList.toggle('flipped', isFlipped);
}

/* ── Progress ───────────────────────────────────────────────────────────────── */
function updateProgress() {
  const total   = words.length;
  const current = currentIdx + 1;
  const pct     = (currentIdx / total) * 100;
  document.getElementById('progressFill').style.width  = pct + '%';
  document.getElementById('progressLabel').textContent = `Word ${current} of ${total}`;
}

/* ── TTS playback ───────────────────────────────────────────────────────────── */
async function playWord() {
  const word = words[currentIdx];
  if (!word) return;
  const btn = document.getElementById('playBtn');
  if (btn) { btn.disabled = true; btn.textContent = '⏳'; }
  try {
    const res = await API.binary('/api/tts', { text: word.word, language });
    if (!res.ok) throw new Error('TTS failed');
    const blob = await res.blob();
    const url  = URL.createObjectURL(blob);
    const audio = new Audio(url);
    audio.onended = () => {
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
    };
    audio.onerror = () => {
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
    };
    // play() rejects (not hangs) when autoplay is blocked — button resets so user can tap
    await audio.play().catch(() => {
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
    });
  } catch {
    if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
  }
}

/* ── Speech recognition ─────────────────────────────────────────────────────── */
function startListening() {
  if (!hasSpeechAPI || !recognition) return;
  if (isListening) {
    stopListening();
    return;
  }
  isListening = true;
  const btn = document.getElementById('listenBtn');
  if (btn) { btn.textContent = '⏹ Stop'; btn.classList.add('listening'); }
  setStatus('Listening…');
  try {
    recognition.start();
  } catch {
    stopListening();
  }
}

function stopListening() {
  isListening = false;
  const btn = document.getElementById('listenBtn');
  if (btn) { btn.textContent = '🎤 Speak'; btn.classList.remove('listening'); }
  try { recognition.stop(); } catch {}
}

/* ── Pronunciation check ────────────────────────────────────────────────────── */
async function checkPronunciation(spoken) {
  const word = words[currentIdx];
  attempts++;
  updateAttemptsLabel();

  try {
    const result = await API.post('/api/vocab/check', { word: word.word, language, spoken });
    if (result.correct) {
      setStatus('Correct! Great pronunciation.', 'correct');
      results.push({ word: word.word, correct: true, attempts });
      setTimeout(() => nextWord(), 1500);
    } else if (attempts < 3) {
      const tip = result.feedback
        ? `Try again: ${result.feedback}`
        : 'Not quite — try again!';
      setStatus(tip, 'feedback');
      setTimeout(() => {
        clearStatus();
        if (hasSpeechAPI) {
          // Re-listen automatically
          startListening();
        }
      }, 2500);
    } else {
      // 3rd attempt failed
      setStatus('Keep practicing! Moving on.', 'incorrect');
      results.push({ word: word.word, correct: false, attempts });
      setTimeout(() => nextWord(), 2000);
    }
  } catch {
    // Network error – treat as incorrect
    if (attempts >= 3) {
      results.push({ word: word.word, correct: false, attempts });
      setTimeout(() => nextWord(), 1500);
    } else {
      setStatus('Could not check — try again.', 'feedback');
    }
  }
}

/* ── Manual fallback ────────────────────────────────────────────────────────── */
function manualResult(correct) {
  const word = words[currentIdx];
  attempts++;
  results.push({ word: word.word, correct, attempts });
  if (correct) {
    setStatus('Got it! Moving on.', 'correct');
  } else {
    setStatus('Noted as a word to review.', 'incorrect');
  }
  setTimeout(() => nextWord(), 1200);
}

/* ── Next word ──────────────────────────────────────────────────────────────── */
function nextWord() {
  currentIdx++;
  if (currentIdx < words.length) {
    renderCard(words[currentIdx]);
    updateProgress();
    setTimeout(() => playWord(), 400);
  } else {
    completeSession();
  }
}

/* ── Complete session ───────────────────────────────────────────────────────── */
async function completeSession() {
  document.getElementById('flashcardContainer').classList.add('hidden');

  try {
    const data = await API.post('/api/vocab/complete', {
      language,
      level,
      topic,
      topic_name: topicName,
      results,
    });

    const weakWords    = data.weak_words   || [];
    const learnedCount = data.learned_count ?? results.filter(r => r.correct).length;
    const fpEarned     = data.fp_earned    ?? results.length * 5;

    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statLearned').textContent = learnedCount;
    document.getElementById('statFP').textContent      = `+${fpEarned} FP`;

    if (weakWords.length) {
      document.getElementById('weakSection').classList.remove('hidden');
      document.getElementById('weakList').innerHTML =
        weakWords.map(w => `<li>${w}</li>`).join('');
    }
  } catch {
    // Still show screen with local data
    const learnedCount = results.filter(r => r.correct).length;
    const fpEarned     = results.length * 5;
    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statLearned').textContent = learnedCount;
    document.getElementById('statFP').textContent      = `+${fpEarned} FP`;
  }

  document.getElementById('completeScreen').classList.remove('hidden');
}

/* ── Practice again ─────────────────────────────────────────────────────────── */
function practiceAgain() {
  window.location.href = window.location.href; // reload same params
}

/* ── Helpers ────────────────────────────────────────────────────────────────── */
function setStatus(msg, type) {
  const el = document.getElementById('vocabStatus');
  el.textContent = msg;
  el.className   = 'vocab-status' + (type ? ' ' + type : '');
}

function clearStatus() {
  const el = document.getElementById('vocabStatus');
  el.textContent = '';
  el.className   = 'vocab-status';
}

function updateAttemptsLabel() {
  const el = document.getElementById('vocabAttempts');
  if (attempts > 0) {
    el.textContent = `Attempt ${attempts} of 3`;
  } else {
    el.textContent = '';
  }
}

function showError(msg) {
  document.getElementById('loadingState').innerHTML =
    `<p style="color:#f87171">${msg}</p><a href="/dashboard.html" class="btn btn-ghost btn-sm" style="margin-top:12px">← Dashboard</a>`;
}
