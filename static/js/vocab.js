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
let isFlipped      = false;
let isListening    = false;
let isPlayingAudio = false;  // true while audio is playing OR during post-play session release delay
let ttsInFlight    = false;  // guard against concurrent playWord() calls
let recognition    = null;
let hasSpeechAPI   = false;

/* ── Language BCP-47 map ─────────────────────────────────────────────────────── */
const LANG_BCP47 = { it: 'it-IT', es: 'es-ES', pt: 'pt-BR' };
const LANG_NAMES = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(async function init() {
  // Detect SpeechRecognition support
  if (window.SpeechRecognition || window.webkitSpeechRecognition) {
    hasSpeechAPI = true;
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
    // Only auto-play if AudioContext is already running (desktop).
    // Mobile browsers always start suspended until a user gesture — skip the
    // wasted TTS fetch and let the user tap Play themselves.
    try {
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      const canAutoPlay = ctx.state === 'running';
      ctx.close();
      if (canAutoPlay) setTimeout(() => playWord(), 600);
    } catch { /* no AudioContext — just skip auto-play */ }
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

// Call with true when audio starts, false when it ends.
// On mobile (iOS/Android) the audio session stays in "playback" mode for ~600ms
// after onended fires — recognition.start() during that window fails silently.
// We keep isPlayingAudio=true for that window so startListening() waits.
function releaseAudioSession() {
  console.log('[vocab] releaseAudioSession: waiting 700ms before enabling mic');
  setTimeout(() => {
    isPlayingAudio = false;
    ttsInFlight    = false;
    const listenBtn = document.getElementById('listenBtn');
    if (listenBtn) listenBtn.disabled = false;
    console.log('[vocab] releaseAudioSession: mic now enabled (isPlayingAudio=false)');
  }, 700);
}

function releaseAudioSessionImmediate() {
  console.log('[vocab] releaseAudioSessionImmediate: mic enabled immediately (no audio played)');
  isPlayingAudio = false;
  ttsInFlight    = false;
  const listenBtn = document.getElementById('listenBtn');
  if (listenBtn) listenBtn.disabled = false;
}

async function playWord() {
  if (ttsInFlight) {
    console.log('[vocab] playWord: skipped — ttsInFlight already true');
    return;
  }
  const word = words[currentIdx];
  if (!word) return;

  console.log('[vocab] playWord: starting fetch for word:', word.word);
  ttsInFlight    = true;
  isPlayingAudio = true;             // block mic immediately
  const btn      = document.getElementById('playBtn');
  const listenBtn = document.getElementById('listenBtn');
  if (btn)       { btn.disabled = true; btn.textContent = '⏳'; }
  if (listenBtn) listenBtn.disabled = true;

  try {
    const res = await API.binary('/api/tts', { text: word.word, language });
    if (!res.ok) throw new Error('TTS failed');
    const blob = await res.blob();
    const url  = URL.createObjectURL(blob);
    const audio = new Audio(url);

    const onPlaybackDone = () => {
      console.log('[vocab] playWord: audio ended/errored — calling releaseAudioSession');
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
      releaseAudioSession();         // keep isPlayingAudio=true for 700ms
    };
    const onPlayBlocked = () => {
      // play() was rejected (autoplay policy) — no audio session was acquired
      console.log('[vocab] playWord: audio.play() blocked by autoplay policy');
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
      releaseAudioSessionImmediate();
    };

    audio.onended = onPlaybackDone;
    audio.onerror = onPlaybackDone;
    console.log('[vocab] playWord: calling audio.play()');
    await audio.play().catch(onPlayBlocked);
  } catch (err) {
    // fetch/blob error — no audio played
    console.log('[vocab] playWord: fetch/blob error:', err);
    if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
    releaseAudioSessionImmediate();
  }
}

/* ── Speech recognition ─────────────────────────────────────────────────────── */
function createRecognition() {
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!SpeechRecognition) return null;
  const r = new SpeechRecognition();
  r.continuous      = false;
  r.interimResults  = false;
  r.lang            = LANG_BCP47[language] || 'it-IT';
  r.onresult = (e) => {
    const transcript = e.results[0][0].transcript;
    console.log('[vocab] recognition.onresult: transcript =', transcript);
    stopListening();
    checkPronunciation(transcript);
  };
  r.onerror = (e) => {
    console.log('[vocab] recognition.onerror: error =', e.error, '| message =', e.message);
    stopListening();
  };
  r.onstart = () => console.log('[vocab] recognition.onstart: mic is active');
  r.onend   = () => {
    console.log('[vocab] recognition.onend: isListening =', isListening);
    if (isListening) stopListening();
  };
  r.onspeechstart = () => console.log('[vocab] recognition.onspeechstart: speech detected');
  r.onspeechend   = () => console.log('[vocab] recognition.onspeechend: speech ended');
  return r;
}

function startListening() {
  console.log('[vocab] startListening called: isListening =', isListening, '| isPlayingAudio =', isPlayingAudio);
  if (!hasSpeechAPI) return;
  if (isListening) {
    stopListening();
    return;
  }
  if (isPlayingAudio) {
    console.log('[vocab] startListening: blocked — audio session still active');
    setStatus('Wait for audio to finish…');
    setTimeout(clearStatus, 1500);
    return;
  }
  // Always create a fresh instance — reusing the same object fails on mobile
  recognition = createRecognition();
  if (!recognition) return;
  isListening = true;
  const btn = document.getElementById('listenBtn');
  if (btn) { btn.textContent = '⏹ Stop'; btn.classList.add('listening'); }
  setStatus('Listening…');
  try {
    console.log('[vocab] calling recognition.start()');
    recognition.start();
  } catch (err) {
    console.log('[vocab] recognition.start() threw:', err);
    stopListening();
  }
}

function stopListening() {
  console.log('[vocab] stopListening called');
  isListening = false;
  const btn = document.getElementById('listenBtn');
  if (btn) { btn.textContent = '🎤 Speak'; btn.classList.remove('listening'); }
  clearStatus();
  try { recognition.stop(); } catch {}
}

/* ── Pronunciation check ────────────────────────────────────────────────────── */
async function checkPronunciation(spoken) {
  const word = words[currentIdx];
  attempts++;
  updateAttemptsLabel();
  setStatus('Checking…');

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
    // Only auto-play if AudioContext is running (i.e. desktop / already unlocked).
    // On mobile the context is still suspended after the previous user gesture — skip.
    try {
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      const canAutoPlay = ctx.state === 'running';
      ctx.close();
      if (canAutoPlay) setTimeout(() => playWord(), 400);
    } catch { /* skip auto-play */ }
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
