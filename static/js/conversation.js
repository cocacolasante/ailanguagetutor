requireAuth();

/* ── Session params from URL ────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const sessionId = params.get('session');
const language  = params.get('language') || 'it';
const topic     = params.get('topic')    || 'general';
const topicName = params.get('topicName') || 'General Conversation';

if (!sessionId) window.location.href = '/dashboard.html';

/* ── State ──────────────────────────────────────────────────────────────────── */
let isSending      = false;
let isRecording    = false;
let recognition    = null;
let currentAudio   = null;
let ttsEnabled     = true;

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
  const playBtn = isUser ? '' : `
    <button class="msg-play-btn" onclick="playMessage(this)" data-text="${escapeAttr(content)}" title="Play audio">
      🔊 Play
    </button>
  `;

  div.innerHTML = `
    <div class="msg-avatar">${avatarContent}</div>
    <div class="msg-body">
      <div class="msg-bubble">${escapeHtml(content)}</div>
      <div class="msg-footer">
        <span class="msg-time">${formatTime()}</span>
        ${playBtn}
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
      <div class="msg-bubble" id="streaming-content"></div>
    </div>
  `;
  container.appendChild(div);
  scrollToBottom();
  return div;
}

function finalizeStreamingMessage(fullText) {
  const msg     = document.getElementById('streaming-msg');
  const content = document.getElementById('streaming-content');
  if (!msg || !content) return;

  msg.classList.remove('streaming');
  msg.id = `msg-${Date.now()}`;

  // Add footer with time and play button
  const body = msg.querySelector('.msg-body');
  const footer = document.createElement('div');
  footer.className = 'msg-footer';
  footer.innerHTML = `
    <span class="msg-time">${formatTime()}</span>
    <button class="msg-play-btn" onclick="playMessage(this)" data-text="${escapeAttr(fullText)}" title="Play audio">
      🔊 Play
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
          contentEl   = document.getElementById('streaming-content');
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

/* ── TTS playback ───────────────────────────────────────────────────────────── */
async function playTTS(text) {
  // Stop current audio if playing
  if (currentAudio) {
    currentAudio.pause();
    currentAudio = null;
    document.querySelectorAll('.msg-play-btn.playing')
      .forEach(b => { b.textContent = '🔊 Play'; b.classList.remove('playing'); });
  }

  const indicator = document.getElementById('audioIndicator');
  indicator.classList.add('show');

  try {
    const res = await API.binary('/api/tts', { text, language });
    if (!res.ok) {
      console.warn('TTS failed:', res.status);
      return;
    }
    const blob = await res.blob();
    const url  = URL.createObjectURL(blob);
    const audio = new Audio(url);
    currentAudio = audio;

    audio.onended = () => {
      URL.revokeObjectURL(url);
      currentAudio = null;
      indicator.classList.remove('show');
    };
    audio.onerror = () => {
      URL.revokeObjectURL(url);
      currentAudio = null;
      indicator.classList.remove('show');
    };
    await audio.play();
  } catch (err) {
    console.warn('TTS error:', err);
    indicator.classList.remove('show');
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

  const wasPlaying = btn.classList.contains('playing');
  // Reset all play buttons
  document.querySelectorAll('.msg-play-btn.playing').forEach(b => {
    b.textContent = '🔊 Play'; b.classList.remove('playing');
  });
  if (currentAudio) { currentAudio.pause(); currentAudio = null; }

  if (!wasPlaying) {
    btn.textContent = '⏹ Stop';
    btn.classList.add('playing');
    playTTS(text).then(() => {
      btn.textContent = '🔊 Play';
      btn.classList.remove('playing');
    });
  }
}

/* ── Voice input ────────────────────────────────────────────────────────────── */
function initSpeechRecognition() {
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!SpeechRecognition) return null;

  const rec = new SpeechRecognition();
  rec.lang = langMeta.bcp47;
  rec.continuous = false;
  rec.interimResults = false;
  rec.maxAlternatives = 1;

  rec.onresult = (event) => {
    const transcript = event.results[0][0].transcript;
    stopRecording();
    if (transcript.trim()) sendMessage(transcript.trim());
  };

  rec.onerror = (event) => {
    console.warn('Speech recognition error:', event.error);
    stopRecording();
    if (event.error !== 'no-speech' && event.error !== 'aborted') {
      appendMessage('assistant', '⚠ Voice input error: ' + event.error + '. Please try typing instead.');
    }
  };

  rec.onend = () => stopRecording();

  return rec;
}

function toggleRecording() {
  if (isRecording) {
    stopRecording();
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
  try {
    recognition.start();
    isRecording = true;
    document.getElementById('micBtn').classList.add('recording');
    document.getElementById('voiceStatus').classList.add('show');
    sendBtn.disabled = true;
    msgInput.disabled = true;
  } catch (err) {
    console.warn('Recording start error:', err);
  }
}

function stopRecording() {
  if (recognition && isRecording) {
    try { recognition.stop(); } catch (_) {}
  }
  isRecording = false;
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
