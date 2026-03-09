requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const language  = params.get('language') || 'it';
const level     = parseInt(params.get('level') || '1', 10);
const topic     = params.get('topic')     || 'general';
const topicName = params.get('topicName') || 'General';

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

// iOS audio: we use AudioContext per-play and close it immediately after playback.
// Closing explicitly signals iOS to deactivate the audio session — the mic
// becomes available for webkitSpeechRecognition without a long wait.
// No getUserMedia: that held the mic exclusively and blocked recognition.
let audioCtx = null;

/* ── BFCache guard ──────────────────────────────────────────────────────────── */
// iOS Safari restores pages from BFCache on back/forward navigation.
// Scalar booleans survive but object references (audioCtx) are nulled,
// causing state mismatches. Reset all state on BFCache restore.
window.addEventListener('pagehide', () => {
  if (audioCtx) { try { audioCtx.close(); } catch {} audioCtx = null; }
});
window.addEventListener('pageshow', (e) => {
  if (e.persisted) {
    console.log('[vocab] BFCache restore — resetting state');
    audioCtx       = null;
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
    const data = await API.post('/api/vocab/session', { language, level, topic });
    words = data.words || [];
    if (!words.length) { showError('No words returned. Please try again.'); return; }

    const langFlag = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' }[language] || '🌐';
    document.getElementById('headerTitle').textContent = `${topicName} — Vocab Builder`;
    document.getElementById('headerSub').textContent =
      `${langFlag} ${LANG_NAMES[language] || language} · Level ${level} · ${words.length} words`;

    document.getElementById('loadingState').classList.add('hidden');
    document.getElementById('flashcardContainer').classList.remove('hidden');

    renderCard(words[0]);
    updateProgress();

    // Desktop: auto-play. iOS: skip (always blocked without prior user gesture).
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
// fromGesture=true only when called from a button tap (needed for AudioContext.resume on iOS).
async function playWord(fromGesture = false) {
  if (ttsInFlight) {
    console.log('[vocab] playWord: skipped — already in flight');
    return;
  }
  if (isListening) {
    console.log('[vocab] playWord: cancelling active recording');
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

  // releaseMic: re-enables the Speak button. On iOS we close the AudioContext
  // first to explicitly signal AVAudioSession deactivation, then wait a short
  // time before enabling the mic. On desktop: immediate.
  const releaseMic = (iosDelay = 300) => {
    if (isIOS && audioCtx) {
      console.log('[vocab] releaseMic: closing AudioContext to deactivate iOS session');
      try { audioCtx.close(); } catch {}
      audioCtx = null;
    }
    const enable = () => {
      isPlayingAudio = false;
      ttsInFlight    = false;
      if (playBtn)   { playBtn.disabled = false; playBtn.textContent = '🔊 Play'; }
      if (listenBtn) listenBtn.disabled = false;
      console.log('[vocab] releaseMic: mic enabled');
    };
    if (isIOS && iosDelay > 0) {
      setTimeout(enable, iosDelay);
    } else {
      enable();
    }
  };

  console.log('[vocab] playWord: fetching TTS for', word.word, '| fromGesture:', fromGesture);
  try {
    const res = await API.binary('/api/tts', { text: word.word, language });
    if (!res.ok) throw new Error('TTS failed');
    const blob = await res.blob();

    if (isIOS && fromGesture) {
      // iOS path: play through AudioContext.
      // After playback we close() the context — this explicitly tells iOS to
      // deactivate the audio session so the mic is free for recognition.
      // No getUserMedia needed (that was blocking recognition by holding the mic).
      try {
        if (!audioCtx || audioCtx.state === 'closed') {
          audioCtx = new (window.AudioContext || window.webkitAudioContext)();
        }
        if (audioCtx.state === 'suspended') await audioCtx.resume();
        console.log('[vocab] playWord: playing via AudioContext, state:', audioCtx.state);

        const arrayBuf = await blob.arrayBuffer();
        const decoded  = await audioCtx.decodeAudioData(arrayBuf);
        await new Promise((resolve, reject) => {
          const src = audioCtx.createBufferSource();
          src.buffer = decoded;
          src.connect(audioCtx.destination);
          src.onended = resolve;
          src.onerror = reject;
          src.start(0);
        });
        console.log('[vocab] playWord: AudioContext playback complete');
        releaseMic(300); // close context + 300ms for iOS session deactivation
      } catch (e) {
        console.log('[vocab] playWord: AudioContext failed, falling back to HTMLAudioElement:', e);
        // Fall back to HTMLAudioElement with a longer delay for iOS session transition.
        const url   = URL.createObjectURL(blob);
        const audio = new Audio(url);
        const done  = () => { URL.revokeObjectURL(url); releaseMic(1200); };
        audio.onended = done;
        audio.onerror = done;
        await audio.play().catch(() => { URL.revokeObjectURL(url); releaseMic(0); });
      }
    } else {
      // Desktop path: HTMLAudioElement, no session concerns.
      console.log('[vocab] playWord: playing via HTMLAudioElement (desktop)');
      const url   = URL.createObjectURL(blob);
      const audio = new Audio(url);
      const done  = () => { URL.revokeObjectURL(url); releaseMic(0); };
      audio.onended = done;
      audio.onerror = done;
      await audio.play().catch(() => { URL.revokeObjectURL(url); releaseMic(0); });
    }
  } catch (err) {
    console.log('[vocab] playWord: error:', err);
    releaseMic(0);
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
      setStatus(result.feedback ? `Try again: ${result.feedback}` : 'Not quite — try again!', 'feedback');
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
  if (audioCtx) { try { audioCtx.close(); } catch {} audioCtx = null; }
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
