requireAuth();

/* ── Session params from URL ────────────────────────────────────────────────── */
const params      = new URLSearchParams(window.location.search);
const sessionId   = params.get('session');
const language    = params.get('language')    || 'it';
const level       = parseInt(params.get('level') || '3', 10);
const topic       = params.get('topic')       || 'general';
const topicName   = params.get('topicName')   || 'General Conversation';
const personality = params.get('personality') || '';

const TTS_PLAYBACK_RATE = 1.0;

if (!sessionId) window.location.href = '/dashboard.html';

/* ── State ──────────────────────────────────────────────────────────────────── */
let isSending         = false;
let isRecording       = false;
let sessionStartTime  = null;
let timerInterval     = null;
let recognition       = null;
let currentTranscript = '';  // accumulated final transcript during recording session
let currentSource     = null;  // AudioBufferSourceNode — mobile-compatible
let audioCtx          = null;  // Web Audio context, unlocked on first user gesture
let ttsEnabled        = true;
let silenceTimer      = null;  // auto-send after speech pause
let onAllAudioDone    = null;  // called once when all TTS audio for a response ends

const SILENCE_TIMEOUT = 1500; // ms of silence before auto-sending

// Cache translations keyed by message text to avoid redundant API calls
const translationCache = new Map();

/* ── Language metadata ──────────────────────────────────────────────────────── */
const LANG_META = {
  it: { flag: '🇮🇹', name: 'Italian',    bcp47: 'it-IT', avatar: '🤌' },
  es: { flag: '🇪🇸', name: 'Spanish',    bcp47: 'es-ES', avatar: '💃' },
  pt: { flag: '🇧🇷', name: 'Portuguese', bcp47: 'pt-BR', avatar: '🎵' },
  fr: { flag: '🇫🇷', name: 'French',     bcp47: 'fr-FR', avatar: '🥐' },
  de: { flag: '🇩🇪', name: 'German',     bcp47: 'de-DE', avatar: '🎻' },
  ja: { flag: '🇯🇵', name: 'Japanese',   bcp47: 'ja-JP', avatar: '🌸' },
  zh: { flag: '🇨🇳', name: 'Chinese',    bcp47: 'zh-CN', avatar: '🐉' },
  ro: { flag: '🇷🇴', name: 'Romanian',   bcp47: 'ro-RO', avatar: '🏰' },
  ru: { flag: '🇷🇺', name: 'Russian',    bcp47: 'ru-RU', avatar: '🎭' },
  sq: { flag: '🇦🇱', name: 'Albanian',   bcp47: 'sq-AL', avatar: '🦅' },
  ar: { flag: '🇸🇦', name: 'Arabic',     bcp47: 'ar-SA', avatar: '🌙' },
};
// Fallback: use the raw language code so the browser still tries the right language
const langMeta = LANG_META[language] || { flag: '🌐', name: language, bcp47: language, avatar: '💬' };
const TOPIC_ICONS = {
  'general':'💬','daily-recap':'📅','future-plans':'🗓️','travel':'✈️',
  'food-dining':'🍽️','shopping':'🛍️','family':'👨‍👩‍👧','health':'🏥',
  'sports':'⚽','culture':'🎭','technology':'💻','cloud':'☁️',
  'marketing':'📊','finance':'💰','education':'📚','news':'📰',
  'work':'💼','entertainment':'🎬','environment':'🌿','home':'🏠',
  'role-restaurant':'🍽️','role-job-interview':'👔','role-airport':'✈️',
  'role-doctor':'🏥','role-business':'💼','role-apartment':'🏠','role-directions':'🗺️',
  'travel-rome':'🇮🇹','travel-barcelona':'🇪🇸','travel-paris':'🇫🇷','travel-tokyo':'🇯🇵','travel-lisbon':'🇵🇹',
  'grammar-vocabulary':'📚','grammar-sentences':'✏️','grammar-pronunciation':'🗣️','grammar-listening':'👂','grammar-writing':'📝',
  'cultural-context':'🏛️','cultural-stories':'📖','cultural-idioms':'💬','cultural-food':'🍜','cultural-history':'🎭',
  'immersion-daily':'🏡','immersion-social':'🥂','immersion-work':'💼','immersion-city':'🏙️','immersion-media':'🎬','immersion-debate':'🗣️',
};
const topicIcon = TOPIC_ICONS[topic] || '💬';

const isImmersion = topic.startsWith('immersion-');

// Immersion mode placeholders in target language
const IMMERSION_PLACEHOLDERS = {
  it: 'Scrivi in italiano…', es: 'Escribe en español…', pt: 'Escreva em português…',
  fr: 'Écrivez en français…', de: 'Schreibe auf Deutsch…', ja: '日本語で書いてください…',
  zh: '用中文写…', ro: 'Scrie în română…', ru: 'Пишите по-русски…',
};

/* ── Init UI ────────────────────────────────────────────────────────────────── */
document.getElementById('headerLang').textContent  = `${langMeta.flag} ${langMeta.name}`;
document.getElementById('headerTopic').textContent = `${topicIcon} ${topicName}`;
document.title = `${langMeta.name} · ${topicName} — Fluentica`;

const ttsToggle = document.getElementById('ttsToggle');
ttsToggle.addEventListener('change', () => { ttsEnabled = ttsToggle.checked; });

/* ── Immersion mode setup ───────────────────────────────────────────────────── */
if (isImmersion) {
  const banner = document.getElementById('immersionBanner');
  if (banner) {
    banner.hidden = false;
    const bannerText = document.getElementById('immersionBannerText');
    if (bannerText) bannerText.textContent = `Immersion Mode — responding only in ${langMeta.name}`;
  }
  const placeholder = IMMERSION_PLACEHOLDERS[language] || `Write in ${langMeta.name}…`;
  document.getElementById('messageInput').placeholder = placeholder;
}

/* ── Text area auto-resize & keyboard submit ────────────────────────────────── */
const msgInput = document.getElementById('messageInput');
const sendBtn  = document.getElementById('sendBtn');

msgInput.addEventListener('input', () => {
  msgInput.style.height = 'auto';
  msgInput.style.height = Math.min(msgInput.scrollHeight, 160) + 'px';
  sendBtn.disabled = msgInput.value.trim() === '' || isSending;
});

msgInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (!sendBtn.disabled) sendMessage();
  }
});

/* ── Message rendering ──────────────────────────────────────────────────────── */
function formatTime(date = new Date()) {
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function appendMessage(role, content, id) {
  const container = document.getElementById('messagesContainer');
  // Hide loading state on first real message
  const loading = document.getElementById('loadingState');
  if (loading) loading.remove();

  const isUser = role === 'user';
  const div    = document.createElement('div');
  div.className = `message ${role}`;
  if (id) div.id = id;

  const avatarContent = isUser ? '👤' : langMeta.avatar;
  const assistantBtns = isUser ? '' : `
    <button class="msg-play-btn" onclick="playMessage(this)" data-text="${escapeAttr(content)}" title="Play audio">
      🔊 Play
    </button>
    ${isImmersion ? '' : `<button class="msg-translate-btn" onclick="translateMessage(this)" data-text="${escapeAttr(content)}" title="Show English translation">
      🌐 Translate
    </button>`}
  `;

  div.innerHTML = `
    <div class="msg-avatar">${avatarContent}</div>
    <div class="msg-body">
      <div class="msg-bubble">${escapeHtml(content)}</div>
      <div class="msg-translation" hidden></div>
      <div class="msg-footer">
        <span class="msg-time">${formatTime()}</span>
        ${assistantBtns}
      </div>
    </div>
  `;
  container.appendChild(div);
  scrollToBottom();
  return div;
}

function appendStreamingMessage() {
  const container = document.getElementById('messagesContainer');
  const loading   = document.getElementById('loadingState');
  if (loading) loading.remove();

  const div = document.createElement('div');
  div.className = 'message assistant streaming';
  div.id = 'streaming-msg';
  div.innerHTML = `
    <div class="msg-avatar">${langMeta.avatar}</div>
    <div class="msg-body">
      <div class="msg-bubble"></div>
    </div>
  `;
  container.appendChild(div);
  scrollToBottom();
  return div;
}

function finalizeStreamingMessage(fullText) {
  const msg     = document.getElementById('streaming-msg');
  if (!msg) return;
  const content = msg.querySelector('.msg-bubble');
  if (!content) return;

  msg.classList.remove('streaming');
  msg.id = `msg-${Date.now()}`;

  // Insert translation placeholder before the footer
  const body = msg.querySelector('.msg-body');
  const translationEl = document.createElement('div');
  translationEl.className = 'msg-translation';
  translationEl.hidden = true;
  body.appendChild(translationEl);

  const footer = document.createElement('div');
  footer.className = 'msg-footer';
  footer.innerHTML = `
    <span class="msg-time">${formatTime()}</span>
    <button class="msg-play-btn" onclick="playMessage(this)" data-text="${escapeAttr(fullText)}" title="Play audio">
      🔊 Play
    </button>
    <button class="msg-translate-btn" onclick="translateMessage(this)" data-text="${escapeAttr(fullText)}" title="Show English translation">
      🌐 Translate
    </button>
  `;
  body.appendChild(footer);
}

function showTypingIndicator() {
  const container = document.getElementById('messagesContainer');
  const loading   = document.getElementById('loadingState');
  if (loading) loading.remove();

  const div = document.createElement('div');
  div.className = 'typing-indicator';
  div.id = 'typing';
  div.innerHTML = `
    <div class="msg-avatar">${langMeta.avatar}</div>
    <div class="typing-dots"><span></span><span></span><span></span></div>
  `;
  container.appendChild(div);
  scrollToBottom();
}

function removeTypingIndicator() {
  document.getElementById('typing')?.remove();
}

function scrollToBottom() {
  const c = document.getElementById('messagesContainer');
  c.scrollTop = c.scrollHeight;
}

/* ── Send message ───────────────────────────────────────────────────────────── */
async function sendMessage(text) {
  const messageText = text ?? msgInput.value.trim();
  if (!messageText || isSending) return;

  onAllAudioDone = null; // cancel any pending mic auto-restart from previous response
  unlockAudio(); // synchronous — inside the tap/click gesture that called sendMessage
  isSending = true;
  sendBtn.disabled = true;
  msgInput.value = '';
  msgInput.style.height = 'auto';

  appendMessage('user', messageText);
  showTypingIndicator();

  try {
    await streamAIResponse(messageText, false);
  } finally {
    isSending = false;
    sendBtn.disabled = msgInput.value.trim() === '';
    msgInput.focus();
  }
}

/* ── Stream AI response ─────────────────────────────────────────────────────── */

// Returns the first complete sentence in text (ends with . ! ? followed by
// whitespace or end-of-string), requiring at least 25 chars before the break
// so we don't fire TTS on a single word like "Ciao!".
function extractFirstSentence(text) {
  if (text.length < 25) return null;
  const m = text.slice(20).search(/[.!?…](?:\s|$)/);
  if (m < 0) return null;
  return text.slice(0, 20 + m + 1).trim();
}

async function streamAIResponse(message, isGreet) {
  removeTypingIndicator();

  const res = await API.stream('/api/conversation/message', {
    session_id: sessionId,
    message:    message || '',
    greet:      isGreet,
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }));
    appendMessage('assistant', '⚠ ' + (err.error || 'Failed to get a response. Please try again.'));
    return;
  }

  const reader  = res.body.getReader();
  const decoder = new TextDecoder();
  let   buffer      = '';
  let   fullText    = '';
  let   streamingEl = null;
  let   contentEl   = null;

  // Two-phase TTS pipeline:
  //   Phase 1 — fires as soon as the first complete sentence arrives; starts
  //             playing immediately, cutting perceived latency.
  //   Phase 2 — remainder of the response; pre-fetched (ElevenLabs request
  //             runs in parallel while phase 1 is playing) so it's ready the
  //             moment phase 1 ends, with no audible gap.
  //   If prefetch fails (null), ttsPhase2Text is set as a fallback and played
  //   via a regular playTTS call instead.
  let ttsStarted    = false;  // phase 1 fired
  let firstSentLen  = 0;      // char length of text sent to phase 1
  let ttsPhase1Done = false;  // phase 1 audio finished playing naturally
  let ttsPhase2Buf  = null;   // decoded AudioBuffer for phase 2, ready to play

  function tryPlayPhase2() {
    if (ttsPhase2Buf && ttsPhase1Done && !currentSource) {
      playDecodedAudio(ttsPhase2Buf);
    }
  }

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop(); // keep incomplete last line

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      const raw = line.slice(6);
      if (!raw) continue;

      let data;
      try { data = JSON.parse(raw); } catch { continue; }

      if (data.done) {
        if (!fullText) {
          // AI returned nothing — clear loading state and show retry prompt
          document.getElementById('loadingState')?.remove();
          document.getElementById('streaming-msg')?.remove();
          appendMessage('assistant', '⚠ No response received. Please try again.');
          return;
        }
        finalizeStreamingMessage(fullText);
        // Schedule mic auto-restart once all TTS audio for this response ends.
        if (ttsEnabled) {
          onAllAudioDone = () => {
            setTimeout(() => {
              if (ttsEnabled && !isSending && !isRecording && !msgInput.value.trim()) {
                startRecording();
              }
            }, 500);
          };
        }
        if (ttsEnabled && ttsStarted) {
          // Pre-fetch phase 2 (remainder after the first sentence) while
          // phase 1 audio is still playing.
          const remainder = fullText.slice(firstSentLen).trim();
          if (remainder) {
            prefetchTTS(remainder).then(buf => {
              if (buf) {
                ttsPhase2Buf = buf;
                tryPlayPhase2();
              } else if (ttsPhase1Done && !currentSource && !isRecording) {
                // Prefetch failed and phase 1 already ended — play directly.
                autoPlayTTS(remainder);
              }
              // If phase 1 is still playing and prefetch failed, phase 2 is
              // skipped — acceptable since it's a rare, short-response case.
            });
          }
        } else if (ttsEnabled && fullText) {
          // Response too short to hit a sentence break — play it all at once.
          autoPlayTTS(fullText);
        }
        return;
      }
      if (data.error) {
        appendMessage('assistant', '⚠ ' + data.error);
        return;
      }
      if (data.content) {
        fullText += data.content;
        if (!streamingEl) {
          streamingEl = appendStreamingMessage();
          contentEl   = streamingEl.querySelector('.msg-bubble');
        }
        contentEl.textContent = fullText;
        scrollToBottom();

        // Phase 1: fire as soon as a complete sentence lands in the stream.
        if (ttsEnabled && !ttsStarted) {
          const sentence = extractFirstSentence(fullText);
          if (sentence) {
            ttsStarted   = true;
            firstSentLen = sentence.length;
            playTTS(sentence, () => {
              ttsPhase1Done = true;
              tryPlayPhase2();
            });
          }
        }
      }
    }
  }

  // Stream ended without a [done] event.
  if (fullText) {
    finalizeStreamingMessage(fullText);
    if (ttsEnabled && !ttsStarted) autoPlayTTS(fullText);
  }
}

/* ── Web Audio context (mobile-compatible) ──────────────────────────────────── */
// Mobile browsers (iOS Safari, Android Chrome) block Audio.play() when called
// from async code after a network request — the user-gesture chain is broken.
// The Web Audio API bypasses this: once the AudioContext is resumed synchronously
// inside a user gesture, it stays unlocked for the lifetime of the page and can
// play audio from any async context.

function getAudioCtx() {
  if (!audioCtx) {
    audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  }
  return audioCtx;
}

// Call this synchronously inside every user-gesture handler (send, mic tap).
function unlockAudio() {
  const ctx = getAudioCtx();
  if (ctx.state === 'suspended') ctx.resume();
  // Play a one-sample silent buffer — fully unlocks audio on iOS Safari.
  const buf = ctx.createBuffer(1, 1, 22050);
  const src = ctx.createBufferSource();
  src.buffer = buf;
  src.connect(ctx.destination);
  src.start(0);
}

function stopCurrentAudio() {
  if (currentSource) {
    currentSource._stopped = true; // tells onended not to trigger chained callbacks
    try { currentSource.stop(); } catch (_) {}
    currentSource = null;
  }
  document.querySelectorAll('.msg-play-btn.playing')
    .forEach(b => { b.textContent = '🔊 Play'; b.classList.remove('playing'); });
}

/* ── TTS text cleanup ───────────────────────────────────────────────────────── */
// Strip annotations that should be read visually but not spoken aloud:
//   (pronunciation hints like "oh-LAH")
//   [grammar/translation notes like "I like food"]
// Also collapse any resulting double spaces.
function cleanForTTS(text) {
  return text
    .replace(/\([^)]*\)/g, '')   // remove (parenthetical content)
    .replace(/\[[^\]]*\]/g, '')  // remove [bracketed content]
    .replace(/  +/g, ' ')
    .trim();
}

/* ── TTS playback ───────────────────────────────────────────────────────────── */
async function playTTS(text, onEnded) {
  text = cleanForTTS(text);
  stopCurrentAudio();

  const indicator = document.getElementById('audioIndicator');
  indicator.classList.add('show');

  try {
    const res = await API.binary('/api/tts', { text, language });
    if (!res.ok) {
      console.warn('TTS failed:', res.status);
      indicator.classList.remove('show');
      onEnded?.();
      return;
    }

    const arrayBuffer = await res.arrayBuffer();
    const ctx = getAudioCtx();
    if (ctx.state === 'suspended') await ctx.resume();

    const audioBuffer = await ctx.decodeAudioData(arrayBuffer);
    const source = ctx.createBufferSource();
    source.buffer = audioBuffer;
    source.playbackRate.value = TTS_PLAYBACK_RATE;
    source.connect(ctx.destination);
    currentSource = source;

    source.onended = () => {
      if (currentSource === source) currentSource = null;
      indicator.classList.remove('show');
      // Don't chain to onEnded if audio was explicitly stopped (user action /
      // new message) — avoids phase-2 playing over top of new content.
      if (!source._stopped) {
        onEnded?.();
        // If onEnded didn't start new audio (e.g. phase 2), all audio is done.
        if (!currentSource) {
          onAllAudioDone?.();
          onAllAudioDone = null;
        }
      }
    };

    source.start(0);
  } catch (err) {
    console.warn('TTS error:', err);
    indicator.classList.remove('show');
    onEnded?.();
  }
}

async function autoPlayTTS(text) {
  text = cleanForTTS(text);
  // Only play the first ~400 chars to keep latency low for long responses
  const excerpt = text.length > 400 ? text.slice(0, text.lastIndexOf(' ', 400)) + '…' : text;
  await playTTS(excerpt);
}

// Fetches and decodes TTS audio without playing it yet.
// Returns an AudioBuffer ready to hand to playDecodedAudio, or null on error.
async function prefetchTTS(text) {
  text = cleanForTTS(text);
  const excerpt = text.length > 350 ? text.slice(0, text.lastIndexOf(' ', 350) || 350) + '…' : text;
  try {
    const res = await API.binary('/api/tts', { text: excerpt, language });
    if (!res.ok) return null;
    const ctx = getAudioCtx();
    return await ctx.decodeAudioData(await res.arrayBuffer());
  } catch { return null; }
}

// Plays a pre-decoded AudioBuffer directly (no fetch latency).
// Assumes the caller has already verified nothing else should be playing.
function playDecodedAudio(audioBuffer) {
  const indicator = document.getElementById('audioIndicator');
  indicator.classList.add('show');
  const ctx = getAudioCtx();
  const source = ctx.createBufferSource();
  source.buffer = audioBuffer;
  source.playbackRate.value = TTS_PLAYBACK_RATE;
  source.connect(ctx.destination);
  currentSource = source;
  source.onended = () => {
    if (currentSource === source) currentSource = null;
    indicator.classList.remove('show');
    if (!source._stopped) {
      onAllAudioDone?.();
      onAllAudioDone = null;
    }
  };
  source.start(0);
}

function playMessage(btn) {
  const text = btn.getAttribute('data-text');
  if (!text) return;

  onAllAudioDone = null; // user is manually playing — don't auto-start mic after
  unlockAudio(); // must be synchronous — this IS the user gesture

  const wasPlaying = btn.classList.contains('playing');
  stopCurrentAudio(); // resets all play buttons + stops any running source

  if (!wasPlaying) {
    btn.textContent = '⏹ Stop';
    btn.classList.add('playing');
    playTTS(text, () => {
      btn.textContent = '🔊 Play';
      btn.classList.remove('playing');
    });
  }
}

/* ── Translation ────────────────────────────────────────────────────────────── */
async function translateMessage(btn) {
  const text = btn.getAttribute('data-text');
  if (!text) return;

  // Find sibling .msg-translation div
  const body = btn.closest('.msg-body');
  const translationEl = body?.querySelector('.msg-translation');
  if (!translationEl) return;

  // Toggle off if already showing
  if (!translationEl.hidden) {
    translationEl.hidden = true;
    btn.textContent = '🌐 Translate';
    return;
  }

  // Show cached translation immediately
  if (translationCache.has(text)) {
    translationEl.textContent = translationCache.get(text);
    translationEl.hidden = false;
    btn.textContent = '🌐 Hide';
    return;
  }

  // Fetch from API
  btn.textContent = '⏳…';
  btn.disabled = true;
  try {
    const data = await API.post('/api/conversation/translate', { text, language });
    if (!data?.translation) throw new Error('Translation failed');
    translationCache.set(text, data.translation);
    translationEl.textContent = data.translation;
    translationEl.hidden = false;
    btn.textContent = '🌐 Hide';
  } catch (err) {
    translationEl.textContent = '⚠ Could not translate. Please try again.';
    translationEl.hidden = false;
    btn.textContent = '🌐 Translate';
    console.warn('Translation error:', err);
  } finally {
    btn.disabled = false;
  }
}

/* ── Voice input ────────────────────────────────────────────────────────────── */
// Mobile-compatible recording strategy:
//   continuous:true + interimResults:true keeps the recognizer alive (mobile browsers
//   frequently fire onend early with continuous:false).  Final transcript chunks are
//   accumulated in currentTranscript; the live preview (final + current interim) is
//   shown in the textarea so the user can see what was captured.  Tapping the mic a
//   second time grabs msgInput.value, stops the recognizer, and sends the text.
function initSpeechRecognition() {
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!SpeechRecognition) return null;

  const rec = new SpeechRecognition();
  rec.lang = langMeta.bcp47;
  rec.continuous = true;      // keeps mic open on mobile; user taps to stop
  rec.interimResults = true;  // live transcript preview while speaking
  rec.maxAlternatives = 3;    // browser returns up to 3 alternatives; we pick the best

  rec.onresult = (event) => {
    if (!isRecording) return;
    let final = '';
    let interim = '';
    for (let i = event.resultIndex; i < event.results.length; i++) {
      // Pick the alternative with the highest confidence score
      let t = event.results[i][0].transcript;
      let bestConf = event.results[i][0].confidence || 0;
      for (let j = 1; j < event.results[i].length; j++) {
        const conf = event.results[i][j].confidence || 0;
        if (conf > bestConf) { bestConf = conf; t = event.results[i][j].transcript; }
      }
      if (event.results[i].isFinal) final += t;
      else interim += t;
    }
    if (final) {
      currentTranscript += final;
      // Speech paused — start countdown to auto-send
      if (silenceTimer) clearTimeout(silenceTimer);
      const vs = document.getElementById('voiceStatus');
      vs.querySelector('span').textContent = 'Got it\u2026 sending';
      silenceTimer = setTimeout(() => {
        silenceTimer = null;
        if (!isRecording) return;
        const text = msgInput.value.trim();
        stopRecording();
        if (text && !isSending) sendMessage(text);
      }, SILENCE_TIMEOUT);
    }
    if (interim) {
      // Still speaking — reset any pending auto-send
      if (silenceTimer) { clearTimeout(silenceTimer); silenceTimer = null; }
      const vs = document.getElementById('voiceStatus');
      vs.querySelector('span').textContent = 'Listening\u2026 tap mic to cancel';
    }
    // Show live preview in the textarea (disabled but writable via JS)
    msgInput.value = (currentTranscript + interim).trim();
    msgInput.style.height = 'auto';
    msgInput.style.height = Math.min(msgInput.scrollHeight, 160) + 'px';
  };

  rec.onerror = (event) => {
    if (event.error === 'aborted') return; // we triggered stop intentionally
    console.warn('Speech recognition error:', event.error);
    if (event.error !== 'no-speech') {
      appendMessage('assistant', '⚠ Voice input error: ' + event.error + '. Please try typing instead.');
    }
    stopRecording();
  };

  rec.onend = () => {
    if (!isRecording) return; // user stopped intentionally — nothing to do
    // Recognition ended unexpectedly (very common on mobile) — restart silently
    try { rec.start(); } catch (e) {
      console.warn('Could not restart recognition:', e);
      stopRecording();
    }
  };

  return rec;
}

function toggleRecording() {
  if (isRecording) {
    // Capture whatever is in the preview textarea, then stop and send
    const text = msgInput.value.trim();
    stopRecording();
    if (text && !isSending) {
      unlockAudio(); // synchronous — still within the tap gesture
      sendMessage(text);
    }
  } else {
    // Manual mic press — stop any playing TTS so the user can speak freely
    stopCurrentAudio();
    onAllAudioDone = null; // cancel any pending auto-restart that would fight the mic
    startRecording();
  }
}

function startRecording() {
  if (!recognition) recognition = initSpeechRecognition();
  if (!recognition) {
    alert('Voice input is not supported in your browser. Please use Chrome or Edge.');
    return;
  }
  currentTranscript = '';
  msgInput.value = '';
  msgInput.style.height = 'auto';
  unlockAudio(); // synchronous — inside the mic button tap gesture
  try {
    recognition.start();
    isRecording = true;
    document.getElementById('micBtn').classList.add('recording');
    const vs = document.getElementById('voiceStatus');
    vs.querySelector('span').textContent = 'Listening\u2026 tap mic to cancel';
    vs.classList.add('show');
    sendBtn.disabled = true;
    msgInput.disabled = true;
  } catch (err) {
    console.warn('Recording start error:', err);
  }
}

function stopRecording() {
  isRecording = false; // set FIRST so onend auto-restart check fails
  if (silenceTimer) { clearTimeout(silenceTimer); silenceTimer = null; }
  try { if (recognition) recognition.stop(); } catch (_) {}
  currentTranscript = '';
  document.getElementById('micBtn').classList.remove('recording');
  document.getElementById('voiceStatus').classList.remove('show');
  msgInput.disabled = false;
  sendBtn.disabled = msgInput.value.trim() === '' || isSending;
  if (!isSending) msgInput.focus();
}

/* ── Timer ──────────────────────────────────────────────────────────────────── */
function startTimer() {
  sessionStartTime = Date.now();
  timerInterval = setInterval(updateTimer, 1000);
}

function updateTimer() {
  const elapsed = Math.floor((Date.now() - sessionStartTime) / 1000);
  const m = Math.floor(elapsed / 60);
  const s = elapsed % 60;
  const el = document.getElementById('convTimer');
  if (el) el.textContent = `${m}:${s.toString().padStart(2, '0')}`;
}

function getElapsedSecs() {
  if (!sessionStartTime) return 0;
  return Math.floor((Date.now() - sessionStartTime) / 1000);
}

/* ── End conversation ───────────────────────────────────────────────────────── */
async function endConversation() {
  if (!sessionId) return;

  if (!confirm('End this conversation and generate a learning summary?')) return;

  // Stop timer and audio
  if (timerInterval) { clearInterval(timerInterval); timerInterval = null; }
  stopCurrentAudio();
  stopRecording();

  const durationSecs = getElapsedSecs();

  // Disable all inputs while ending
  document.getElementById('sendBtn').disabled = true;
  document.getElementById('micBtn').disabled = true;
  document.querySelector('.btn-danger')?.setAttribute('disabled', '');

  // Show loading in messages
  const container = document.getElementById('messagesContainer');
  const loadingDiv = document.createElement('div');
  loadingDiv.className = 'end-session-loading';
  loadingDiv.innerHTML = `<div class="spinner"></div><span>Generating your learning summary…</span>`;
  container.appendChild(loadingDiv);
  container.scrollTop = container.scrollHeight;

  try {
    const data = await API.post('/api/conversation/end', {
      session_id:    sessionId,
      duration_secs: durationSecs,
    });

    if (!data) return;
    sessionStorage.setItem('summary_record_' + data.record_id, JSON.stringify(data));
    window.location.href = `/summary.html?record=${data.record_id}`;
  } catch (err) {
    console.error('Failed to end conversation:', err);
    loadingDiv.remove();
    document.getElementById('sendBtn').disabled = false;
    document.getElementById('micBtn').disabled = false;
    document.querySelector('.btn-danger')?.removeAttribute('disabled');
    alert('Failed to generate summary. Please try again.');
  }
}

/* ── New session ────────────────────────────────────────────────────────────── */
function newConversation() {
  window.location.href = '/dashboard.html';
}

/* ── HTML / attr escaping ───────────────────────────────────────────────────── */
function escapeHtml(str) {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/\n/g, '<br>');
}
function escapeAttr(str) {
  return str.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

/* ── Boot: request AI greeting ──────────────────────────────────────────────── */
// iOS Safari (and some Android browsers) block AudioContext.resume() unless called
// synchronously inside a user-gesture handler.  We show a "tap to start" overlay so
// that unlockAudio() fires inside the tap, then begin the greeting.  This is a no-op
// cost on desktop (one extra click) but is required for mobile audio to work.
async function startGreeting() {
  try {
    await streamAIResponse('', true); // greet = true
  } catch (err) {
    console.error('Greeting failed:', err);
    document.getElementById('loadingState')?.remove();
    appendMessage('assistant', `Ciao! Sono pronto per praticare con te. Come stai oggi?`);
  }
}

(function boot() {
  const overlay  = document.getElementById('startOverlay');
  const startBtn = document.getElementById('startBtn');
  overlay.hidden = false;
  startBtn.addEventListener('click', () => {
    unlockAudio();      // synchronous — inside the tap gesture
    overlay.hidden = true;
    startTimer();
    startGreeting();
  }, { once: true });
})();


