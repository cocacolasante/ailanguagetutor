requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params    = new URLSearchParams(window.location.search);
const language  = params.get('language')  || 'it';
const level     = parseInt(params.get('level') || '3', 10);
const topic     = params.get('topic')     || 'general';
const topicName = params.get('topicName') || 'General';

/* ── State ──────────────────────────────────────────────────────────────────── */
let sessionId        = null;
let isSending        = false;
let sessionStartTime = null;
let timerInterval    = null;
let allMisspellings  = []; // accumulated across session, sent with complete
// Track the last user bubble element for retroactive misspelling highlights
let lastUserBubble   = null;
let lastUserText     = '';

/* ── Audio (Web Audio API — same approach as vocab.js) ───────────────────────── */
let audioCtx      = null;
let currentSource = null;

function getAudioCtx() {
  if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  return audioCtx;
}

function stopCurrentAudio() {
  if (currentSource) {
    try { currentSource.stop(); } catch (_) {}
    currentSource = null;
  }
}

async function playText(text, btnEl) {
  if (!text) return;
  stopCurrentAudio();

  const origLabel = btnEl.textContent;
  btnEl.disabled = true;
  btnEl.textContent = '⏳';

  try {
    const ctx = getAudioCtx();
    if (ctx.state === 'suspended') await ctx.resume();

    const res = await API.binary('/api/tts', { text, language });
    if (!res.ok) throw new Error('TTS failed');

    const arrayBuf    = await res.arrayBuffer();
    const audioBuffer = await ctx.decodeAudioData(arrayBuf);

    stopCurrentAudio();
    const source = ctx.createBufferSource();
    source.buffer = audioBuffer;
    source.connect(ctx.destination);
    currentSource = source;

    btnEl.textContent = '⏹';
    btnEl.disabled = false;
    btnEl.onclick = () => { stopCurrentAudio(); resetPlayBtn(btnEl, origLabel); };

    source.onended = () => {
      if (currentSource === source) currentSource = null;
      resetPlayBtn(btnEl, origLabel);
    };
    source.start(0);
  } catch (err) {
    console.error('[writing] TTS error:', err);
    resetPlayBtn(btnEl, origLabel);
  }
}

function resetPlayBtn(btnEl, label) {
  btnEl.textContent = label;
  btnEl.disabled    = false;
  btnEl.onclick     = null; // will be re-bound by appendMessage
}

/* ── Init ───────────────────────────────────────────────────────────────────── */
async function init() {
  document.getElementById('headerSub').textContent =
    `${language.toUpperCase()} · Level ${level} · ${topicName}`;
  document.title = `Writing Coach · ${topicName} — Fluentica`;

  try {
    const data = await API.post('/api/writing/session', { language, level, topic, topicName });
    sessionId = data.session_id;

    document.getElementById('loadingState').classList.add('hidden');
    const chat = document.getElementById('chatContainer');
    chat.classList.remove('hidden');
    chat.style.display = 'flex';

    appendMessage('assistant', data.first_message);
    startTimer();
    // Don't auto-focus on mobile — iOS Safari locks up touch/scroll events
    // when focus() is called programmatically during page load.
    if (window.innerWidth > 768) {
      document.getElementById('msgInput').focus();
    }
  } catch (err) {
    document.getElementById('loadingState').innerHTML =
      `<p style="color:var(--text-2)">Failed to start session: ${escapeHtml(err.message || 'Unknown error')}</p>
       <a href="/dashboard.html" class="btn btn-ghost btn-sm">← Back to Dashboard</a>`;
  }
}

/* ── Timer ──────────────────────────────────────────────────────────────────── */
function startTimer() {
  sessionStartTime = Date.now();
  timerInterval = setInterval(() => {
    const elapsed = Math.floor((Date.now() - sessionStartTime) / 1000);
    document.getElementById('timerDisplay').textContent = formatTime(elapsed);
  }, 1000);
}

function formatTime(secs) {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

function elapsedSecs() {
  if (!sessionStartTime) return 0;
  return Math.floor((Date.now() - sessionStartTime) / 1000);
}

/* ── Append message ─────────────────────────────────────────────────────────── */
function appendMessage(role, content, misspellingAnnotations = []) {
  const container = document.getElementById('messagesContainer');

  const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const avatar  = role === 'assistant' ? '🤖' : '🧑';

  const bubble = document.createElement('div');
  bubble.className = `message ${role}`;

  let bubbleHtml;
  if (role === 'user' && misspellingAnnotations.length > 0) {
    bubbleHtml = highlightMisspellings(escapeHtml(content), misspellingAnnotations);
  } else {
    bubbleHtml = escapeHtml(content);
  }

  bubble.innerHTML = `
    <div class="msg-avatar">${avatar}</div>
    <div class="msg-body">
      <div class="msg-bubble">${bubbleHtml}</div>
      ${role === 'assistant' ? '<div class="msg-actions"></div>' : ''}
      <div class="msg-time">${timeStr}</div>
    </div>`;

  container.appendChild(bubble);
  scrollToBottom();

  // Track last user bubble for retroactive update
  if (role === 'user') {
    lastUserBubble = bubble.querySelector('.msg-bubble');
    lastUserText   = content;
  }

  // Wire up action buttons for AI messages
  if (role === 'assistant') {
    const actionsEl = bubble.querySelector('.msg-actions');

    // Play button
    const playBtn = document.createElement('button');
    playBtn.className   = 'msg-action-btn';
    playBtn.textContent = '🔊 Play';
    playBtn.onclick = () => playText(content, playBtn);
    actionsEl.appendChild(playBtn);

    // Translate button
    const transBtn = document.createElement('button');
    transBtn.className   = 'msg-action-btn';
    transBtn.textContent = '🌐 Translate';
    transBtn.onclick = async () => {
      transBtn.disabled    = true;
      transBtn.textContent = '…';
      try {
        const data = await API.post('/api/conversation/translate', { text: content, language });
        // Show translation inline below the bubble
        const existing = bubble.querySelector('.msg-translation');
        if (existing) {
          existing.remove();
          transBtn.textContent = '🌐 Translate';
          transBtn.disabled    = false;
          return;
        }
        const transEl = document.createElement('div');
        transEl.className   = 'msg-translation';
        transEl.textContent = data.translation || data.text || '';
        bubble.querySelector('.msg-body').insertBefore(transEl, actionsEl);
        transBtn.textContent = '🌐 Hide';
        transBtn.disabled    = false;
        scrollToBottom();
      } catch {
        transBtn.textContent = '🌐 Translate';
        transBtn.disabled    = false;
      }
    };
    actionsEl.appendChild(transBtn);
  }

  return bubble;
}

/* ── Highlight misspellings ─────────────────────────────────────────────────── */
// annotations: array of "wrong → correct (note)" strings
function highlightMisspellings(escapedContent, annotations) {
  let result = escapedContent;
  for (const annotation of annotations) {
    const arrowIdx = annotation.indexOf('→');
    if (arrowIdx < 0) continue;
    const wrong = annotation.substring(0, arrowIdx).trim();
    if (!wrong) continue;
    // Escape the wrong word for use in regex
    const escaped = wrong.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const re = new RegExp('\\b' + escaped + '\\b', 'gi');
    const tooltipText = annotation.replace(/"/g, '&quot;');
    result = result.replace(re, match =>
      `<span class="misspelling-highlight" title="${tooltipText}">${match}</span>`
    );
  }
  return result;
}

/* ── Send message ───────────────────────────────────────────────────────────── */
async function sendMessage() {
  if (isSending || !sessionId) return;
  const input = document.getElementById('msgInput');
  const text  = input.value.trim();
  if (!text) return;

  isSending = true;
  input.value = '';
  input.style.height = '';
  document.getElementById('sendBtn').disabled = true;

  // Append user bubble immediately
  appendMessage('user', text);

  // Show typing indicator
  const typingEl = showTypingIndicator();

  try {
    const data = await API.post('/api/writing/message', {
      session_id: sessionId,
      message:    text,
    });

    typingEl.remove();

    // Retroactively highlight misspellings on the user bubble
    if (data.misspellings && data.misspellings.length > 0) {
      if (lastUserBubble) {
        lastUserBubble.innerHTML = highlightMisspellings(escapeHtml(lastUserText), data.misspellings);
      }
      allMisspellings.push(...data.misspellings);
    }

    appendMessage('assistant', data.reply);
  } catch (err) {
    typingEl.remove();
    appendMessage('assistant', '⚠️ ' + escapeHtml(err.message || 'Failed to get reply'));
  } finally {
    isSending = false;
    document.getElementById('sendBtn').disabled = false;
    if (window.innerWidth > 768) input.focus();
  }
}

/* ── Typing indicator ───────────────────────────────────────────────────────── */
function showTypingIndicator() {
  const container = document.getElementById('messagesContainer');
  const el = document.createElement('div');
  el.className = 'message assistant';
  el.innerHTML = `
    <div class="msg-avatar">🤖</div>
    <div class="msg-body">
      <div class="typing-indicator">
        <div class="typing-dot"></div>
        <div class="typing-dot"></div>
        <div class="typing-dot"></div>
      </div>
    </div>`;
  container.appendChild(el);
  scrollToBottom();
  return el;
}

/* ── End session ────────────────────────────────────────────────────────────── */
async function endSession() {
  if (!sessionId) return;
  if (!confirm('End this writing session and get your summary?')) return;

  clearInterval(timerInterval);
  const duration = elapsedSecs();

  // Disable inputs
  document.getElementById('msgInput').disabled = true;
  document.getElementById('sendBtn').disabled   = true;
  document.querySelector('.writing-end-row button').disabled = true;

  // Show loading message in thread
  const loadingEl = document.createElement('div');
  loadingEl.style.cssText = 'text-align:center;padding:20px;color:var(--text-2)';
  loadingEl.innerHTML = '<div class="spinner" style="margin:0 auto 8px"></div><p>Generating your summary…</p>';
  document.getElementById('messagesContainer').appendChild(loadingEl);
  scrollToBottom();

  try {
    const data = await API.post('/api/writing/complete', {
      session_id:    sessionId,
      duration_secs: duration,
      topic_name:    topicName,
      misspellings:  allMisspellings,
    });

    // Stash full record data for summary.js
    sessionStorage.setItem('summary_record_' + data.record_id, JSON.stringify({
      id:                    data.record_id,
      language:              data.language,
      topic:                 data.topic,
      topic_name:            data.topic_name,
      level:                 data.level,
      personality:           data.personality,
      message_count:         data.message_count,
      duration_secs:         data.duration_secs,
      fp_earned:             data.fp_earned,
      new_streak:            data.new_streak,
      new_achievements:      data.new_achievements,
      summary:               data.summary,
      topics_discussed:      data.topics_discussed,
      vocabulary_learned:    data.vocabulary_learned,
      grammar_corrections:   data.grammar_corrections,
      suggested_next_lessons: data.suggested_next_lessons,
      misspellings:          data.misspellings,
    }));

    window.location.href = '/summary.html?record=' + data.record_id;
  } catch (err) {
    loadingEl.innerHTML = `<p style="color:var(--text-2)">Failed to save session: ${escapeHtml(err.message || 'Unknown error')}</p>`;
  }
}

/* ── Utility ────────────────────────────────────────────────────────────────── */
function scrollToBottom() {
  const c = document.getElementById('messagesContainer');
  c.scrollTop = c.scrollHeight;
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/* ── Input event listeners ──────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  const input = document.getElementById('msgInput');

  // Auto-resize
  input.addEventListener('input', () => {
    input.style.height = '';
    input.style.height = Math.min(input.scrollHeight, 140) + 'px';
  });

  // Enter to send (Shift+Enter for newline)
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  });
});

/* ── Boot ───────────────────────────────────────────────────────────────────── */
init();
