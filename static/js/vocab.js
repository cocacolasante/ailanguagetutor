requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params      = new URLSearchParams(window.location.search);
const language    = params.get('language')     || 'it';
const level       = parseInt(params.get('level') || '1', 10);
const topic       = params.get('topic')         || 'general';
const topicName   = params.get('topicName')     || 'General';
const mistakesMode = params.get('mistakes_mode') === 'true';

/* ── Platform detection ─────────────────────────────────────────────────────── */
const isIOS = /iPhone|iPad|iPod/.test(navigator.userAgent);
console.log('[vocab] isIOS:', isIOS);

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

/* ── Web Audio context (mirrors conversation.js) ────────────────────────────── */
// A single AudioContext lives for the page lifetime and is never closed.
// This keeps iOS in a shared PlayAndRecord audio session so the mic remains
// accessible immediately after playback — no session transition needed.
// HTMLAudioElement must NOT be used for TTS on iOS: it puts the session into
// exclusive Playback mode, which blocks webkitSpeechRecognition for minutes.
let audioCtx     = null;
let currentSource = null;

function getAudioCtx() {
  if (!audioCtx) {
    audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  }
  return audioCtx;
}

// Call synchronously inside every user-gesture handler before any await.
// Resumes the context and plays a silent sample — fully unlocks iOS audio.
function unlockAudio() {
  const ctx = getAudioCtx();
  if (ctx.state === 'suspended') ctx.resume();
  const buf = ctx.createBuffer(1, 1, 22050);
  const src = ctx.createBufferSource();
  src.buffer = buf;
  src.connect(ctx.destination);
  src.start(0);
}

function stopCurrentAudio() {
  if (currentSource) {
    currentSource._stopped = true;
    try { currentSource.stop(); } catch (_) {}
    currentSource = null;
  }
}

/* ── BFCache guard ──────────────────────────────────────────────────────────── */
window.addEventListener('pageshow', (e) => {
  if (e.persisted) {
    console.log('[vocab] BFCache restore — resetting state');
    // audioCtx reference may be stale after BFCache; null it so getAudioCtx recreates.
    audioCtx       = null;
    currentSource  = null;
    ttsInFlight    = false;
    isPlayingAudio = false;
    isListening    = false;
    const playBtn   = document.getElementById('playBtn');
    const listenBtn = document.getElementById('listenBtn');
    if (playBtn)   { playBtn.disabled = false; playBtn.textContent = '🔊 Play'; }
    if (listenBtn) listenBtn.disabled = false;
  }
});

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
    const sessionBody = { language, level, topic };
    if (mistakesMode) sessionBody.mistakes_mode = true;
    const data = await API.post('/api/vocab/session', sessionBody);
    words = data.words || [];
    if (!words.length) {
      const msg = data.message || 'No words returned. Please try again.';
      showError(msg);
      return;
    }

    const langFlag = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' }[language] || '🌐';
    document.getElementById('headerTitle').textContent = mistakesMode
      ? 'Improving Weak Words'
      : `${topicName} — Vocab Builder`;
    document.getElementById('headerSub').textContent =
      `${langFlag} ${LANG_NAMES[language] || language} · Level ${level} · ${words.length} words`;

    document.getElementById('loadingState').classList.add('hidden');
    document.getElementById('flashcardContainer').classList.remove('hidden');

    renderCard(words[0]);
    updateProgress();

    // Desktop: auto-play. iOS: skip (blocked without prior user gesture).
    if (!isIOS) setTimeout(() => playWord(false), 600);
  } catch (err) {
    showError('Failed to load vocabulary. ' + (err.message || ''));
  }
}

/* ── Render card ────────────────────────────────────────────────────────────── */
function renderCard(word) {
  document.getElementById('cardWord').textContent        = word.word;
  document.getElementById('cardPhonetic').textContent    = word.phonetic ? `[ ${word.phonetic} ]` : '';
  document.getElementById('cardTranslation').textContent = word.translation;
  document.getElementById('flashcard').classList.remove('flipped');
  isFlipped = false;
  attempts  = 0;
  clearStatus();
  updateAttemptsLabel();
}

/* ── Flip card ──────────────────────────────────────────────────────────────── */
function flipCard() {
  isFlipped = !isFlipped;
  document.getElementById('flashcard').classList.toggle('flipped', isFlipped);
}

/* ── Progress ───────────────────────────────────────────────────────────────── */
function updateProgress() {
  const pct = (currentIdx / words.length) * 100;
  document.getElementById('progressFill').style.width  = pct + '%';
  document.getElementById('progressLabel').textContent = `Word ${currentIdx + 1} of ${words.length}`;
}

/* ── TTS playback ───────────────────────────────────────────────────────────── */
// fromGesture=true when called from a button tap.
// unlockAudio() MUST be called synchronously (before any await) to unlock iOS audio.
// We use AudioContext (Web Audio API) for all playback — this keeps iOS in a shared
// PlayAndRecord session so the mic is immediately available after audio ends.
async function playWord(fromGesture = false) {
  if (ttsInFlight) {
    console.log('[vocab] playWord: skipped — already in flight');
    return;
  }

  // Unlock iOS audio synchronously while we're still inside the gesture handler.
  if (fromGesture) unlockAudio();

  const wasListening = isListening;
  if (isListening) {
    console.log('[vocab] playWord: stopping active recording first');
    stopListening();
  }

  const word = words[currentIdx];
  if (!word) return;

  ttsInFlight    = true;
  isPlayingAudio = true;
  const playBtn   = document.getElementById('playBtn');
  const listenBtn = document.getElementById('listenBtn');
  if (playBtn)   { playBtn.disabled = true; playBtn.textContent = '⏳'; }
  if (listenBtn) listenBtn.disabled = true;

  const enable = () => {
    isPlayingAudio = false;
    ttsInFlight    = false;
    if (playBtn)   { playBtn.disabled = false; playBtn.textContent = '🔊 Play'; }
    if (listenBtn) listenBtn.disabled = false;
    console.log('[vocab] playWord: mic enabled');
  };

  // If we just stopped a recording, give iOS a moment to exit Record mode
  // before the TTS fetch begins (prevents playback silently failing).
  if (isIOS && wasListening) {
    console.log('[vocab] playWord: waiting 400ms for iOS to exit Record session');
    await new Promise(r => setTimeout(r, 400));
  }

  console.log('[vocab] playWord: fetching TTS for', word.word, '| fromGesture:', fromGesture);
  try {
    const res = await API.binary('/api/tts', { text: word.word, language });
    if (!res.ok) throw new Error('TTS failed');

    const ctx = getAudioCtx();
    if (ctx.state === 'suspended') await ctx.resume();

    const arrayBuf    = await res.arrayBuffer();
    const audioBuffer = await ctx.decodeAudioData(arrayBuf);

    stopCurrentAudio(); // cancel any previously playing audio

    const source = ctx.createBufferSource();
    source.buffer = audioBuffer;
    source.connect(ctx.destination);
    currentSource = source;

    // Safety: if onended never fires, release after 15s.
    const safetyTimer = setTimeout(() => {
      console.log('[vocab] playWord: safety timeout — releasing mic');
      currentSource = null;
      clearStatus();
      enable();
    }, 15000);

    source.onended = () => {
      clearTimeout(safetyTimer);
      if (currentSource === source) currentSource = null;
      console.log('[vocab] playWord: audio ended');
      // With AudioContext the iOS session stays in PlayAndRecord, so a short
      // 500ms pause is enough for iOS to fully settle before recognition starts.
      if (isIOS) {
        setStatus('Tap Speak when ready…');
        setTimeout(() => { clearStatus(); enable(); }, 500);
      } else {
        enable();
      }
    };

    console.log('[vocab] playWord: starting AudioContext playback');
    source.start(0);
  } catch (err) {
    console.log('[vocab] playWord: error:', err);
    enable();
  }
}

/* ── Speech recognition ─────────────────────────────────────────────────────── */
function createRecognition() {
  const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!SR) return null;
  const r = new SR();
  r.continuous     = false;
  r.interimResults = false;
  r.lang           = LANG_BCP47[language] || 'it-IT';

  let gotResult = false;

  r.onstart       = () => { gotResult = false; console.log('[vocab] recognition.onstart'); };
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
    const wasListening = isListening;
    stopListening();
    if (wasListening && e.error !== 'aborted') {
      setStatus('Mic error — tap Speak to try again.', 'feedback');
    }
  };

  r.onend = () => {
    console.log('[vocab] recognition.onend — gotResult:', gotResult, 'isListening:', isListening);
    if (isListening) {
      stopListening();
      if (!gotResult) {
        setStatus("Didn't catch that — tap Speak to try again.", 'feedback');
      }
    }
  };
  return r;
}

// Synchronous — no async/await before recognition.start() so user-gesture chain is intact.
function startListening() {
  console.log('[vocab] startListening: isListening=', isListening, 'isPlayingAudio=', isPlayingAudio);
  if (!hasSpeechAPI) return;
  if (isListening) { stopListening(); return; }
  if (isPlayingAudio) {
    console.log('[vocab] startListening: blocked — audio still active');
    setStatus('Wait for audio to finish…');
    setTimeout(clearStatus, 1500);
    return;
  }

  // Unlock audio synchronously — keeps iOS AudioContext running in PlayAndRecord mode.
  unlockAudio();

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
    setStatus('Could not start mic — tap Speak to retry.', 'feedback');
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

/* ── Mid-lesson persistence ──────────────────────────────────────────────────── */
function saveWordResult(word, correct) {
  API.post('/api/vocab/word-result', { word, language, correct }).catch(() => {});
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
      saveWordResult(word.word, true);
      setTimeout(() => nextWord(), 1500);
    } else if (attempts < 3) {
      setStatus(result.feedback ? `Try again: ${result.feedback}` : 'Not quite — try again!', 'feedback');
    } else {
      setStatus('Keep practicing! Moving on.', 'incorrect');
      results.push({ word: word.word, correct: false, attempts });
      saveWordResult(word.word, false);
      setTimeout(() => nextWord(), 2000);
    }
  } catch {
    if (attempts >= 3) {
      results.push({ word: word.word, correct: false, attempts });
      saveWordResult(word.word, false);
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
  saveWordResult(word.word, correct);
  setStatus(correct ? 'Got it! Moving on.' : 'Noted as a word to review.', correct ? 'correct' : 'incorrect');
  setTimeout(() => nextWord(), 1200);
}

/* ── Next word ──────────────────────────────────────────────────────────────── */
function nextWord() {
  currentIdx++;
  if (currentIdx < words.length) {
    renderCard(words[currentIdx]);
    updateProgress();
    if (!isIOS) setTimeout(() => playWord(false), 400);
  } else {
    completeSession();
  }
}

/* ── Complete session ───────────────────────────────────────────────────────── */
async function completeSession() {
  stopCurrentAudio();
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
    document.getElementById('statTotal').textContent   = results.length;
    document.getElementById('statLearned').textContent = results.filter(r => r.correct).length;
    document.getElementById('statFP').textContent      = `+${results.length * 5} FP`;
  }
  document.getElementById('completeScreen').classList.remove('hidden');
}

/* ── Practice again ─────────────────────────────────────────────────────────── */
function practiceAgain() { window.location.href = window.location.href; }

/* ── Helpers ─────────────────────────────────────────────────────────────────── */
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
  document.getElementById('vocabAttempts').textContent = attempts > 0 ? `Attempt ${attempts} of 3` : '';
}
function showError(msg) {
  document.getElementById('loadingState').innerHTML =
    `<p style="color:#f87171">${msg}</p><a href="/dashboard.html" class="btn btn-ghost btn-sm" style="margin-top:12px">← Dashboard</a>`;
}
