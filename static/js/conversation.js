requireAuth();

/* ── Session params from URL ────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const sessionId = params.get('session');
const language  = params.get('language') || 'it';
const topic     = params.get('topic')    || 'general';
const topicName = params.get('topicName') || 'General Conversation';

if (!sessionId) window.location.href = '/dashboard.html';

/* ── State ──────────────────────────────────────────────────────────────────── */
let isSending         = false;
let isRecording       = false;
let recognition       = null;
let currentTranscript = '';  // accumulated final transcript during recording session
let currentSource     = null;  // AudioBufferSourceNode — mobile-compatible
let audioCtx          = null;  // Web Audio context, unlocked on first user gesture
let ttsEnabled        = true;

// Cache translations keyed by message text to avoid redundant API calls
const translationCache = new Map();

/* ── Language metadata ──────────────────────────────────────────────────────── */
const LANG_META = {
  it: { flag: '🇮🇹', name: 'Italian',    bcp47: 'it-IT', avatar: '🤌' },
  es: { flag: '🇪🇸', name: 'Spanish',    bcp47: 'es-ES', avatar: '💃' },
  pt: { flag: '🇧🇷', name: 'Portuguese', bcp47: 'pt-BR', avatar: '🎵' },
};
const langMeta  = LANG_META[language] || LANG_META.it;
const TOPIC_ICONS = {
  'general':'💬','daily-recap':'📅','future-plans':'🗓️','travel':'✈️',
  'food-dining':'🍽️','shopping':'🛍️','family':'👨‍👩‍👧','health':'🏥',
  'sports':'⚽','culture':'🎭','technology':'💻','cloud':'☁️',
  'marketing':'📊','finance':'💰','education':'📚','news':'📰',
  'work':'💼','entertainment':'🎬','environment':'🌿','home':'🏠',
};
const topicIcon = TOPIC_ICONS[topic] || '💬';

/* ── Init UI ────────────────────────────────────────────────────────────────── */
document.getElementById('headerLang').textContent  = `${langMeta.flag} ${langMeta.name}`;
document.getElementById('headerTopic').textContent = `${topicIcon} ${topicName}`;
document.title = `${langMeta.name} · ${topicName} — LinguaAI`;

const ttsToggle = document.getElementById('ttsToggle');
ttsToggle.addEventListener('change', () => { ttsEnabled = ttsToggle.checked; });

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
    <button class="msg-translate-btn" onclick="translateMessage(this)" data-text="${escapeAttr(content)}" title="Show English translation">
      🌐 Translate
    </button>
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
  let   buffer  = '';
  let   fullText = '';
  let   streamingEl = null;
  let   contentEl   = null;

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
        finalizeStreamingMessage(fullText);
        if (ttsEnabled && fullText) autoPlayTTS(fullText);
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
      }
    }
  }

  // Stream ended without [done]
  if (fullText) {
    finalizeStreamingMessage(fullText);
    if (ttsEnabled) autoPlayTTS(fullText);
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
    try { currentSource.stop(); } catch (_) {}
    currentSource = null;
  }
  document.querySelectorAll('.msg-play-btn.playing')
    .forEach(b => { b.textContent = '🔊 Play'; b.classList.remove('playing'); });
}

/* ── TTS playback ───────────────────────────────────────────────────────────── */
async function playTTS(text, onEnded) {
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
    source.connect(ctx.destination);
    currentSource = source;

    source.onended = () => {
      if (currentSource === source) currentSource = null;
      indicator.classList.remove('show');
      onEnded?.();
    };

    source.start(0);
  } catch (err) {
    console.warn('TTS error:', err);
    indicator.classList.remove('show');
    onEnded?.();
  }
}

async function autoPlayTTS(text) {
  // Only play the first ~400 chars to keep latency low for long responses
  const excerpt = text.length > 400 ? text.slice(0, text.lastIndexOf(' ', 400)) + '…' : text;
  await playTTS(excerpt);
}

function playMessage(btn) {
  const text = btn.getAttribute('data-text');
  if (!text) return;

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
  rec.maxAlternatives = 1;

  rec.onresult = (event) => {
    if (!isRecording) return;
    let final = '';
    let interim = '';
    for (let i = event.resultIndex; i < event.results.length; i++) {
      const t = event.results[i][0].transcript;
      if (event.results[i].isFinal) final += t;
      else interim += t;
    }
    if (final) currentTranscript += final;
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
    vs.querySelector('span').textContent = 'Listening\u2026 tap mic to send';
    vs.classList.add('show');
    sendBtn.disabled = true;
    msgInput.disabled = true;
  } catch (err) {
    console.warn('Recording start error:', err);
  }
}

function stopRecording() {
  isRecording = false; // set FIRST so onend auto-restart check fails
  try { if (recognition) recognition.stop(); } catch (_) {}
  currentTranscript = '';
  document.getElementById('micBtn').classList.remove('recording');
  document.getElementById('voiceStatus').classList.remove('show');
  msgInput.disabled = false;
  sendBtn.disabled = msgInput.value.trim() === '' || isSending;
  if (!isSending) msgInput.focus();
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
(async function boot() {
  try {
    await streamAIResponse('', true); // greet = true
  } catch (err) {
    console.error('Greeting failed:', err);
    document.getElementById('loadingState')?.remove();
    appendMessage('assistant', `Ciao! Sono pronto per praticare con te. Come stai oggi?`);
  }
})();
