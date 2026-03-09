requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const language  = params.get('language') || 'it';
const level     = parseInt(params.get('level') || '1', 10);
const topic     = params.get('topic')     || 'general';
const topicName = params.get('topicName') || 'General';

/* ── State ──────────────────────────────────────────────────────────────────── */
let words      = [];
let currentIdx = 0;
let attempts   = 0;
let results    = [];
let isFlipped      = false;
let isListening    = false;
let isPlayingAudio = false;
let ttsInFlight    = false;
let recognition    = null;
let hasSpeechAPI   = false;

// iOS fix: keep a silent MediaStream alive so iOS stays in "play and record"
// audio session mode. Without this, each audio playback kicks the session to
// "playback-only", and recognition.start() silently fails until iOS transitions back.
let keepAliveMicStream = null;

/* ── Language BCP-47 map ─────────────────────────────────────────────────────── */
const LANG_BCP47 = { it: 'it-IT', es: 'es-ES', pt: 'pt-BR' };
const LANG_NAMES = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(async function init() {
  if (window.SpeechRecognition || window.webkitSpeechRecognition) {
    hasSpeechAPI = true;
  } else {
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
    if (!words.length) { showError('No words returned. Please try again.'); return; }

    const langFlag = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' }[language] || '🌐';
    document.getElementById('headerTitle').textContent = `${topicName} — Vocab Builder`;
    document.getElementById('headerSub').textContent   =
      `${langFlag} ${LANG_NAMES[language] || language} · Level ${level} · ${words.length} words`;

    document.getElementById('loadingState').classList.add('hidden');
    document.getElementById('flashcardContainer').classList.remove('hidden');

    renderCard(words[0]);
    updateProgress();

    // Only auto-play on desktop (AudioContext already running).
    // Mobile starts suspended until a user gesture — skip wasted TTS fetch.
    try {
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      const canAutoPlay = ctx.state === 'running';
      ctx.close();
      if (canAutoPlay) setTimeout(() => playWord(), 600);
    } catch { /* skip auto-play */ }
  } catch (err) {
    showError('Failed to load vocabulary. ' + (err.message || ''));
  }
}

/* ── Render card ────────────────────────────────────────────────────────────── */
function renderCard(word) {
  document.getElementById('cardWord').textContent        = word.word;
  document.getElementById('cardPhonetic').textContent    = word.phonetic ? `[ ${word.phonetic} ]` : '';
  document.getElementById('cardTranslation').textContent = word.translation;
  const card = document.getElementById('flashcard');
  card.classList.remove('flipped');
  isFlipped = false;
  attempts  = 0;
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
  if (ttsInFlight) {
    console.log('[vocab] playWord: skipped — ttsInFlight');
    return;
  }

  // If recording when Play is tapped, cancel it first
  if (isListening) {
    console.log('[vocab] playWord: stopping active recording before play');
    stopListening();
  }

  const word = words[currentIdx];
  if (!word) return;

  ttsInFlight    = true;
  isPlayingAudio = true;
  const btn       = document.getElementById('playBtn');
  const listenBtn = document.getElementById('listenBtn');
  if (btn)       { btn.disabled = true; btn.textContent = '⏳'; }
  if (listenBtn) listenBtn.disabled = true;

  console.log('[vocab] playWord: fetching TTS for', word.word);
  try {
    const res = await API.binary('/api/tts', { text: word.word, language });
    if (!res.ok) throw new Error('TTS failed');
    const blob  = await res.blob();
    const url   = URL.createObjectURL(blob);
    const audio = new Audio(url);

    const onPlaybackDone = () => {
      console.log('[vocab] playWord: audio ended — releasing audio session');
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
      // With keep-alive mic stream active, iOS stays in play+record mode and
      // recognition can start immediately. Keep a short safety delay anyway.
      setTimeout(() => {
        isPlayingAudio = false;
        ttsInFlight    = false;
        if (listenBtn) listenBtn.disabled = false;
        console.log('[vocab] playWord: mic re-enabled');
      }, keepAliveMicStream ? 100 : 800);
    };

    const onPlayBlocked = () => {
      console.log('[vocab] playWord: autoplay blocked');
      URL.revokeObjectURL(url);
      if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
      isPlayingAudio = false;
      ttsInFlight    = false;
      if (listenBtn) listenBtn.disabled = false;
    };

    audio.onended = onPlaybackDone;
    audio.onerror = onPlaybackDone;
    console.log('[vocab] playWord: calling audio.play()');
    await audio.play().catch(onPlayBlocked);
  } catch (err) {
    console.log('[vocab] playWord: error:', err);
    if (btn) { btn.disabled = false; btn.textContent = '🔊 Play'; }
    isPlayingAudio = false;
    ttsInFlight    = false;
    if (listenBtn) listenBtn.disabled = false;
  }
}

/* ── Keep-alive mic stream (iOS audio session fix) ───────────────────────────── */
// Called on the first user tap of Speak. Acquires a silent mic stream that keeps
// iOS's audio session in "play and record" mode for the rest of the session.
async function ensureKeepAliveMic() {
  if (keepAliveMicStream) return;
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) return;
  try {
    keepAliveMicStream = await navigator.mediaDevices.getUserMedia({ audio: true });
    console.log('[vocab] keep-alive mic stream acquired — iOS will stay in playAndRecord mode');
  } catch (err) {
    console.log('[vocab] keep-alive mic failed (non-fatal):', err);
  }
}

/* ── Speech recognition ─────────────────────────────────────────────────────── */
function createRecognition() {
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!SpeechRecognition) return null;
  const r = new SpeechRecognition();
  r.continuous     = false;
  r.interimResults = false;
  r.lang           = LANG_BCP47[language] || 'it-IT';

  let gotResult = false;

  r.onstart = () => {
    gotResult = false;
    console.log('[vocab] recognition.onstart: mic active');
  };
  r.onspeechstart = () => console.log('[vocab] recognition.onspeechstart');
  r.onspeechend   = () => console.log('[vocab] recognition.onspeechend');
  r.onresult = (e) => {
    gotResult = true;
    const transcript = e.results[0][0].transcript;
    console.log('[vocab] recognition.onresult:', transcript);
    stopListening();
    checkPronunciation(transcript);
  };
  r.onerror = (e) => {
    console.log('[vocab] recognition.onerror:', e.error, e.message);
    stopListening();
    if (e.error !== 'aborted') {
      setStatus('Mic error — tap Speak to try again.', 'feedback');
    }
  };
  r.onend = () => {
    console.log('[vocab] recognition.onend — gotResult:', gotResult, '| isListening:', isListening);
    if (isListening) {
      // Ended without a result (silent fail or timeout)
      stopListening();
      if (!gotResult) {
        setStatus("Didn't catch that — tap Speak to try again.", 'feedback');
      }
    }
  };
  return r;
}

async function startListening() {
  console.log('[vocab] startListening: isListening=', isListening, 'isPlayingAudio=', isPlayingAudio);
  if (!hasSpeechAPI) return;
  if (isListening) {
    stopListening();
    return;
  }
  if (isPlayingAudio) {
    console.log('[vocab] startListening: blocked by active audio');
    setStatus('Wait for audio to finish…');
    setTimeout(clearStatus, 1500);
    return;
  }

  // Acquire keep-alive stream on first tap (user gesture context).
  // This is the key fix for iOS: holds audio session in play+record mode.
  await ensureKeepAliveMic();

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
  console.log('[vocab] stopListening');
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
      const tip = result.feedback ? `Try again: ${result.feedback}` : 'Not quite — try again!';
      setStatus(tip, 'feedback');
    } else {
      setStatus('Keep practicing! Moving on.', 'incorrect');
      results.push({ word: word.word, correct: false, attempts });
      setTimeout(() => nextWord(), 2000);
    }
  } catch {
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
  setStatus(correct ? 'Got it! Moving on.' : 'Noted as a word to review.',
            correct ? 'correct' : 'incorrect');
  setTimeout(() => nextWord(), 1200);
}

/* ── Next word ──────────────────────────────────────────────────────────────── */
function nextWord() {
  currentIdx++;
  if (currentIdx < words.length) {
    renderCard(words[currentIdx]);
    updateProgress();
    // Only auto-play on desktop (AudioContext running = already unlocked by user gesture).
    try {
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      const canAutoPlay = ctx.state === 'running';
      ctx.close();
      if (canAutoPlay) setTimeout(() => playWord(), 400);
    } catch { /* skip */ }
  } else {
    completeSession();
  }
}

/* ── Complete session ───────────────────────────────────────────────────────── */
async function completeSession() {
  // Release keep-alive stream when done
  if (keepAliveMicStream) {
    keepAliveMicStream.getTracks().forEach(t => t.stop());
    keepAliveMicStream = null;
  }

  document.getElementById('flashcardContainer').classList.add('hidden');

  try {
    const data = await API.post('/api/vocab/complete', {
      language, level, topic, topic_name: topicName, results,
    });

    const weakWords    = data.weak_words   || [];
    const learnedCount = data.learned_count ?? results.filter(r => r.correct).length;
    const fpEarned     = data.fp_earned    ?? results.length * 5;

    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statLearned').textContent = learnedCount;
    document.getElementById('statFP').textContent      = `+${fpEarned} FP`;

    if (weakWords.length) {
      document.getElementById('weakSection').classList.remove('hidden');
      document.getElementById('weakList').innerHTML = weakWords.map(w => `<li>${w}</li>`).join('');
    }
  } catch {
    const learnedCount = results.filter(r => r.correct).length;
    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statLearned').textContent = learnedCount;
    document.getElementById('statFP').textContent      = `+${results.length * 5} FP`;
  }

  document.getElementById('completeScreen').classList.remove('hidden');
}

/* ── Practice again ─────────────────────────────────────────────────────────── */
function practiceAgain() {
  window.location.href = window.location.href;
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
  el.textContent = attempts > 0 ? `Attempt ${attempts} of 3` : '';
}

function showError(msg) {
  document.getElementById('loadingState').innerHTML =
    `<p style="color:#f87171">${msg}</p><a href="/dashboard.html" class="btn btn-ghost btn-sm" style="margin-top:12px">← Dashboard</a>`;
}
