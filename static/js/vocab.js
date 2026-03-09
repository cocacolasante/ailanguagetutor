requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const language  = params.get('language') || 'it';
const level     = parseInt(params.get('level') || '1', 10);
const topic     = params.get('topic')     || 'general';
const topicName = params.get('topicName') || 'General';

/* ── Platform detection ─────────────────────────────────────────────────────── */
// iOS (Safari + Chrome-on-iOS) uses WebKit's exclusive audio session model.
// Playing HTMLAudioElement switches the session to "Playback-only", which
// silently kills webkitSpeechRecognition. We use AudioContext for playback on
// iOS so we can hold a "PlayAndRecord" session throughout.
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

/* ── iOS audio pipeline ─────────────────────────────────────────────────────── */
// On iOS: one AudioContext + a silent mic source keep the session in
// "PlayAndRecord" mode so recognition works immediately after playback.
let audioCtx       = null;   // shared AudioContext for the whole session
let micStream      = null;   // getUserMedia stream (kept alive)
let pipelineReady  = false;

async function initAudioPipeline() {
  if (pipelineReady) return true;
  try {
    // getUserMedia must be called inside a user-gesture handler
    micStream = await navigator.mediaDevices.getUserMedia({ audio: true });

    audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    if (audioCtx.state === 'suspended') await audioCtx.resume();

    // Connect mic → silent gain → destination.
    // This registers the mic with the AudioContext so iOS sets
    // AVAudioSession category to PlayAndRecord for this context.
    const micSrc  = audioCtx.createMediaStreamSource(micStream);
    const silence = audioCtx.createGain();
    silence.gain.value = 0;
    micSrc.connect(silence);
    silence.connect(audioCtx.destination);

    pipelineReady = true;
    console.log('[vocab] iOS audio pipeline ready (PlayAndRecord mode)');
    return true;
  } catch (err) {
    console.log('[vocab] initAudioPipeline failed:', err);
    audioCtx      = null;
    micStream     = null;
    pipelineReady = false;
    return false;
  }
}

async function playViaAudioContext(blob) {
  if (audioCtx.state === 'suspended') await audioCtx.resume();
  const arrayBuf = await blob.arrayBuffer();
  const decoded  = await audioCtx.decodeAudioData(arrayBuf);
  return new Promise((resolve, reject) => {
    const src = audioCtx.createBufferSource();
    src.buffer = decoded;
    src.connect(audioCtx.destination);
    src.onended = resolve;
    src.onerror = reject;
    src.start(0);
  });
}

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

    // Desktop: auto-play via HTMLAudioElement (no session conflict on desktop).
    // iOS: skip — autoplay is always blocked and would waste a TTS call.
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
// fromGesture=true when called directly from a button tap (needed for iOS pipeline init).
// fromGesture=false for auto-play (desktop only).
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

  // On iOS, initialize the audio pipeline on first user-gesture tap of Play.
  // This locks the AVAudioSession into PlayAndRecord mode so the mic works
  // immediately after audio ends — no timing delays required.
  if (isIOS && fromGesture && !pipelineReady) {
    console.log('[vocab] playWord: initializing iOS audio pipeline');
    await initAudioPipeline();
  }

  const releaseMic = () => {
    isPlayingAudio = false;
    ttsInFlight    = false;
    if (playBtn)   { playBtn.disabled = false; playBtn.textContent = '🔊 Play'; }
    if (listenBtn) listenBtn.disabled = false;
    console.log('[vocab] playWord: done — mic enabled');
  };

  console.log('[vocab] playWord: fetching TTS for', word.word, '| pipeline:', pipelineReady);
  try {
    const res = await API.binary('/api/tts', { text: word.word, language });
    if (!res.ok) throw new Error('TTS failed');
    const blob = await res.blob();

    if (pipelineReady && audioCtx) {
      // iOS path: play through the shared AudioContext.
      // The mic source connected to this context keeps iOS in PlayAndRecord mode,
      // so recognition.start() works immediately when audio ends.
      console.log('[vocab] playWord: playing via AudioContext');
      try {
        await playViaAudioContext(blob);
      } catch (e) {
        console.log('[vocab] AudioContext playback error:', e);
      }
      releaseMic();
    } else {
      // Desktop path: HTMLAudioElement (no iOS session conflict on desktop).
      console.log('[vocab] playWord: playing via HTMLAudioElement');
      const url   = URL.createObjectURL(blob);
      const audio = new Audio(url);
      const done  = () => { URL.revokeObjectURL(url); releaseMic(); };
      audio.onended = done;
      audio.onerror = done;
      await audio.play().catch(() => { URL.revokeObjectURL(url); releaseMic(); });
    }
  } catch (err) {
    console.log('[vocab] playWord: error:', err);
    releaseMic();
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
      // Ended without a result (silent fail or no-speech timeout)
      stopListening();
      if (!gotResult) {
        setStatus("Didn't catch that — tap Speak to try again.", 'feedback');
      }
    }
  };
  return r;
}

// Kept synchronous — no async/await so user-gesture chain is never broken.
function startListening() {
  console.log('[vocab] startListening: isListening=', isListening, 'isPlayingAudio=', isPlayingAudio, 'pipeline=', pipelineReady);
  if (!hasSpeechAPI) return;
  if (isListening) { stopListening(); return; }
  if (isPlayingAudio) {
    console.log('[vocab] startListening: blocked — audio active');
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
    // Desktop: auto-play. iOS: let the user tap Play (autoplay always blocked).
    if (!isIOS) setTimeout(() => playWord(false), 400);
  } else {
    completeSession();
  }
}

/* ── Complete session ───────────────────────────────────────────────────────── */
async function completeSession() {
  if (micStream) { micStream.getTracks().forEach(t => t.stop()); micStream = null; }
  if (audioCtx)  { try { audioCtx.close(); } catch {} audioCtx = null; }

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
