requireAuth();

/* ── Session params from URL ────────────────────────────────────────────────── */
const params      = new URLSearchParams(window.location.search);
const sessionId   = params.get('session');
const language    = params.get('language')  || 'it';
const level       = parseInt(params.get('level') || '3', 10);
const topic       = params.get('topic')     || 'general';
const topicName   = params.get('topicName') || 'General Conversation';
const personality = params.get('personality') || '';

if (!sessionId) window.location.href = '/dashboard.html';

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
};
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

/* ── State ──────────────────────────────────────────────────────────────────── */
let ws                  = null;   // ElevenLabs WebSocket
let micStream           = null;   // MediaStream
let micAudioCtx         = null;   // AudioContext for microphone capture
let pcmPlayer           = null;   // PCMPlayer for agent audio playback
let sessionStartTime    = null;
let timerInterval       = null;
let conversationId      = null;   // ElevenLabs conversation ID
let transcript          = [];     // [{role, content}] collected from WS events
let userMsgCount        = 0;
let currentAgentMsgEl   = null;   // live streaming agent bubble
let agentResponseFinal  = '';     // final text from agent_response event
let isMuted             = false;
let isAgentSpeaking     = false;  // true while agent audio is playing

const translationCache = new Map();

/* ── Init UI ────────────────────────────────────────────────────────────────── */
document.getElementById('headerLang').textContent  = `${langMeta.flag} ${langMeta.name}`;
document.getElementById('headerTopic').textContent = `${topicIcon} ${topicName}`;
document.title = `${langMeta.name} · ${topicName} — LinguaAI`;

// Always explicitly set banner state — never rely on HTML default
const immersionBanner = document.getElementById('immersionBanner');
if (immersionBanner) {
  immersionBanner.hidden = !isImmersion;
  if (isImmersion) {
    const txt = document.getElementById('immersionBannerText');
    if (txt) txt.textContent = `Immersion Mode — responding only in ${langMeta.name}`;
    const IMMERSION_PLACEHOLDERS = {
      it: 'Scrivi in italiano…', es: 'Escribe en español…', pt: 'Escreva em português…',
      fr: 'Écrivez en français…', de: 'Schreibe auf Deutsch…', ja: '日本語で書いてください…',
      zh: '用中文写…', ro: 'Scrie în română…', ru: 'Пишите по-русски…',
    };
    document.getElementById('messageInput').placeholder =
      IMMERSION_PLACEHOLDERS[language] || `Write in ${langMeta.name}…`;
  }
}

/* ── Mute toggle (hides agent audio volume) ─────────────────────────────────── */
const ttsToggle = document.getElementById('ttsToggle');
ttsToggle.addEventListener('change', () => {
  isMuted = !ttsToggle.checked;
  if (pcmPlayer) pcmPlayer.setMuted(isMuted);
});

/* ── Text area auto-resize & keyboard submit ────────────────────────────────── */
const msgInput = document.getElementById('messageInput');
const sendBtn  = document.getElementById('sendBtn');

msgInput.addEventListener('input', () => {
  msgInput.style.height = 'auto';
  msgInput.style.height = Math.min(msgInput.scrollHeight, 160) + 'px';
  sendBtn.disabled = msgInput.value.trim() === '';
});

msgInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (!sendBtn.disabled) sendTextMessage(msgInput.value.trim());
  }
});

/* ── Send text message via WebSocket ────────────────────────────────────────── */
function sendMessage() {
  const text = msgInput.value.trim();
  if (!text) return;
  sendTextMessage(text);
}

function sendTextMessage(text) {
  if (!text) return;
  msgInput.value = '';
  msgInput.style.height = 'auto';
  sendBtn.disabled = true;

  // Show user message immediately in UI
  appendMessage('user', text);
  transcript.push({ role: 'user', content: text });
  userMsgCount++;

  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ user_message: text }));
  }
}

/* ── PCM Player ─────────────────────────────────────────────────────────────── */
class PCMPlayer {
  constructor(sampleRate = 16000) {
    this.ctx       = new (window.AudioContext || window.webkitAudioContext)({ sampleRate });
    this.gainNode  = this.ctx.createGain();
    this.gainNode.connect(this.ctx.destination);
    this.nextTime  = this.ctx.currentTime;
    this.sources   = [];
    this.isMuted   = false;
  }

  resume() {
    if (this.ctx.state === 'suspended') this.ctx.resume();
  }

  setMuted(muted) {
    this.isMuted = muted;
    this.gainNode.gain.setTargetAtTime(muted ? 0 : 1, this.ctx.currentTime, 0.05);
  }

  enqueue(base64) {
    if (this.isMuted) return; // skip decoding entirely when muted
    try {
      const binary = atob(base64);
      const bytes  = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);

      const int16   = new Int16Array(bytes.buffer);
      const float32 = new Float32Array(int16.length);
      for (let i = 0; i < int16.length; i++) float32[i] = int16[i] / 32768.0;

      const buf = this.ctx.createBuffer(1, float32.length, this.ctx.sampleRate);
      buf.getChannelData(0).set(float32);

      const src = this.ctx.createBufferSource();
      src.buffer = buf;
      src.connect(this.gainNode);

      const startAt = Math.max(this.ctx.currentTime, this.nextTime);
      src.start(startAt);
      this.nextTime = startAt + buf.duration;
      this.sources.push(src);

      showAudioIndicator(true);
      src.onended = () => {
        this.sources = this.sources.filter(s => s !== src);
        if (this.sources.length === 0) {
          showAudioIndicator(false);
          setMicTurn(true); // agent done → user's turn
        }
      };
    } catch (e) {
      console.warn('PCMPlayer.enqueue error:', e);
    }
  }

  // Stop all scheduled/playing sources immediately (for interruptions)
  clear() {
    this.sources.forEach(s => { try { s.stop(0); } catch {} });
    this.sources  = [];
    this.nextTime = this.ctx.currentTime;
    showAudioIndicator(false);
    setMicTurn(true);
  }
}

/* ── Microphone capture ─────────────────────────────────────────────────────── */
async function startMicrophone() {
  const stream = await navigator.mediaDevices.getUserMedia({
    audio: {
      sampleRate:       16000,
      channelCount:     1,
      echoCancellation: true,
      noiseSuppression: true,
    },
  });

  micStream = stream;
  micAudioCtx = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 16000 });
  const source = micAudioCtx.createMediaStreamSource(stream);

  await micAudioCtx.audioWorklet.addModule('/js/pcm-worklet.js');
  const worklet = new AudioWorkletNode(micAudioCtx, 'pcm-capture');
  source.connect(worklet);
  // Route worklet output through a silent gain to destination.
  // Some browsers only schedule worklet processing when the graph reaches destination.
  const silentGain = micAudioCtx.createGain();
  silentGain.gain.value = 0;
  worklet.connect(silentGain);
  silentGain.connect(micAudioCtx.destination);

  worklet.port.onmessage = (e) => {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ user_audio_chunk: arrayBufferToBase64(e.data) }));
  };
}

function stopMicrophone() {
  if (micStream) {
    micStream.getTracks().forEach(t => t.stop());
    micStream = null;
  }
  if (micAudioCtx) {
    micAudioCtx.close().catch(() => {});
    micAudioCtx = null;
  }
}

function arrayBufferToBase64(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

/* ── WebSocket connection ───────────────────────────────────────────────────── */
async function connectToAgent() {
  const data = await API.post('/api/conversation/agent-url', { session_id: sessionId });
  if (!data?.signed_url) throw new Error('No signed URL received from server.');

  ws = new WebSocket(data.signed_url);

  ws.onopen = () => {
    // Send per-session config override — this is what makes each session unique
    ws.send(JSON.stringify({
      type: 'conversation_initiation_client_data',
      conversation_config_override: {
        agent: {
          prompt:        { prompt: data.system_prompt },
          first_message: data.first_message || '',
          language:      data.language || 'en',
        },
        tts: {
          voice_id: data.voice_id,
        },
      },
    }));
  };

  ws.onmessage = handleWSMessage;

  ws.onclose = (e) => {
    console.log('ElevenLabs WebSocket closed:', e.code, e.reason);
    stopMicrophone();
    document.getElementById('micBtn').classList.remove('recording');
    document.getElementById('voiceStatus').classList.remove('show');

    if (e.code === 1002 || (e.reason && e.reason.toLowerCase().includes('quota'))) {
      document.getElementById('loadingState')?.remove();
      appendMessage('assistant', '⚠ Your ElevenLabs voice quota has been reached. Please log in to elevenlabs.io and check your usage, then refresh to try again.');
    }
  };

  ws.onerror = (e) => {
    console.error('ElevenLabs WebSocket error:', e);
    appendMessage('assistant', '⚠ Connection error. Please refresh and try again.');
  };

  // Start microphone after WebSocket is open — starts in "waiting for agent" state
  await startMicrophone();
  setMicTurn(false); // agent will speak first; flip to user turn when audio finishes
}

/* ── WebSocket message handling ─────────────────────────────────────────────── */
function handleWSMessage(event) {
  let data;
  try { data = JSON.parse(event.data); } catch { return; }

  switch (data.type) {
    case 'conversation_initiation_metadata':
      conversationId = data.conversation_initiation_metadata_event?.conversation_id;
      // Session is live — remove loading state
      document.getElementById('loadingState')?.remove();
      break;

    case 'audio':
      if (data.audio_event?.audio_base_64 && pcmPlayer) {
        if (!isAgentSpeaking) setMicTurn(false); // agent started — flip to agent turn
        pcmPlayer.enqueue(data.audio_event.audio_base_64);
      }
      break;

    case 'internal_tentative_agent_response': {
      // Streaming preview — update live bubble
      const partial = data.tentative_agent_response_internal_event?.tentative_agent_response;
      if (partial) updateAgentStreamingBubble(partial);
      break;
    }

    case 'agent_response': {
      // Final complete text for this turn
      const text = data.agent_response_event?.agent_response;
      if (text) {
        agentResponseFinal = text;
        finalizeAgentBubble(text);
        transcript.push({ role: 'assistant', content: text });
        // When muted, src.onended never fires so we flip turn here instead.
        // Also covers cases where audio finishes before agent_response arrives.
        if (isMuted || !pcmPlayer || pcmPlayer.sources.length === 0) {
          setMicTurn(true);
        }
      }
      break;
    }

    case 'user_transcript': {
      const userText = data.user_transcription_event?.user_transcript;
      if (userText && userText.trim()) {
        appendMessage('user', userText.trim());
        transcript.push({ role: 'user', content: userText.trim() });
        userMsgCount++;
        sendBtn.disabled = msgInput.value.trim() === '';
        // Briefly show "got it" then switch to agent-speaking state
        const vs = document.getElementById('voiceStatus');
        if (vs) vs.querySelector('span').textContent = 'Got it…';
        document.getElementById('micBtn')?.classList.remove('recording');
      }
      break;
    }

    case 'interruption':
      // User interrupted the agent — stop queued audio and finalize partial bubble
      if (pcmPlayer) pcmPlayer.clear();
      if (currentAgentMsgEl && agentResponseFinal) {
        finalizeAgentBubble(agentResponseFinal);
      } else if (currentAgentMsgEl) {
        // Partial message — finalize what we have from the streaming bubble
        const bubbleText = currentAgentMsgEl.querySelector('.msg-bubble')?.textContent || '';
        if (bubbleText) finalizeAgentBubble(bubbleText);
      }
      break;

    case 'ping':
      ws.send(JSON.stringify({ type: 'pong', event_id: data.ping_event?.event_id }));
      break;

    case 'agent_response_correction':
      // Corrected response after interruption
      const corrected = data.agent_response_correction_event?.corrected_agent_response;
      if (corrected) agentResponseFinal = corrected;
      break;
  }
}

/* ── Agent message bubbles ──────────────────────────────────────────────────── */
function updateAgentStreamingBubble(text) {
  if (!currentAgentMsgEl) {
    const container = document.getElementById('messagesContainer');
    document.getElementById('loadingState')?.remove();

    const div = document.createElement('div');
    div.className = 'message assistant streaming';
    div.innerHTML = `
      <div class="msg-avatar">${langMeta.avatar}</div>
      <div class="msg-body">
        <div class="msg-bubble"></div>
      </div>
    `;
    container.appendChild(div);
    currentAgentMsgEl = div;
    scrollToBottom();
  }
  const bubble = currentAgentMsgEl.querySelector('.msg-bubble');
  if (bubble) { bubble.textContent = text; scrollToBottom(); }
}

function finalizeAgentBubble(text) {
  if (!currentAgentMsgEl) {
    // No streaming bubble — create a complete message
    appendMessage('assistant', text);
    currentAgentMsgEl = null;
    agentResponseFinal = '';
    return;
  }

  const msg = currentAgentMsgEl;
  msg.classList.remove('streaming');

  const bubble = msg.querySelector('.msg-bubble');
  if (bubble) bubble.textContent = text;

  const body = msg.querySelector('.msg-body');

  const translationEl = document.createElement('div');
  translationEl.className = 'msg-translation';
  translationEl.hidden = true;
  body.appendChild(translationEl);

  const footer = document.createElement('div');
  footer.className = 'msg-footer';
  footer.innerHTML = `
    <span class="msg-time">${formatTime()}</span>
    <button class="msg-play-btn" onclick="playTTSMessage(this)" data-text="${escapeAttr(text)}" title="Re-play audio">
      🔊 Play
    </button>
    ${isImmersion ? '' : `<button class="msg-translate-btn" onclick="translateMessage(this)" data-text="${escapeAttr(text)}" title="Show English translation">
      🌐 Translate
    </button>`}
  `;
  body.appendChild(footer);

  currentAgentMsgEl  = null;
  agentResponseFinal = '';
  scrollToBottom();
}

/* ── Message rendering helpers ──────────────────────────────────────────────── */
function appendMessage(role, content) {
  const container = document.getElementById('messagesContainer');
  document.getElementById('loadingState')?.remove();

  const isUser = role === 'user';
  const div    = document.createElement('div');
  div.className = `message ${role}`;

  const avatarContent = isUser ? '👤' : langMeta.avatar;
  const assistantBtns = isUser ? '' : `
    <button class="msg-play-btn" onclick="playTTSMessage(this)" data-text="${escapeAttr(content)}" title="Re-play audio">
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

function scrollToBottom() {
  const c = document.getElementById('messagesContainer');
  c.scrollTop = c.scrollHeight;
}

function formatTime(date = new Date()) {
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

/* ── Audio indicator ────────────────────────────────────────────────────────── */
function showAudioIndicator(show) {
  document.getElementById('audioIndicator')?.classList.toggle('show', show);
}

/* ── Mic turn state ─────────────────────────────────────────────────────────── */
// Controls the mic button visual + voice status label to reflect whose turn it is.
// The mic stream itself is always active; ElevenLabs VAD handles actual turn detection.
function setMicTurn(userTurn) {
  isAgentSpeaking = !userTurn;
  const micBtn = document.getElementById('micBtn');
  const vs     = document.getElementById('voiceStatus');
  if (!micBtn || !vs) return;

  if (userTurn) {
    // Always un-mute and re-enable tracks when handing turn back to user
    if (micStream) {
      micMuted = false;
      micStream.getAudioTracks().forEach(t => { t.enabled = true; });
    }
    micBtn.classList.add('recording');
    vs.querySelector('span').textContent = 'Your turn — speak now';
    vs.classList.add('show');
  } else {
    micBtn.classList.remove('recording');
    vs.querySelector('span').textContent = 'Agent speaking…';
  }
}

/* ── Manual TTS playback (re-play button on messages) ───────────────────────── */
// Uses the existing /api/tts endpoint so users can replay any message
let ttsAudioCtx    = null;
let ttsCurrentSrc  = null;

function getTTSCtx() {
  if (!ttsAudioCtx) ttsAudioCtx = new (window.AudioContext || window.webkitAudioContext)();
  return ttsAudioCtx;
}

async function playTTSMessage(btn) {
  const text = btn.getAttribute('data-text');
  if (!text) return;

  const wasPlaying = btn.classList.contains('playing');

  // Stop any currently playing manual TTS
  if (ttsCurrentSrc) {
    ttsCurrentSrc._stopped = true;
    try { ttsCurrentSrc.stop(); } catch {}
    ttsCurrentSrc = null;
  }
  document.querySelectorAll('.msg-play-btn.playing').forEach(b => {
    b.textContent = '🔊 Play'; b.classList.remove('playing');
  });

  if (wasPlaying) return; // toggle off

  btn.textContent = '⏹ Stop';
  btn.classList.add('playing');

  try {
    const res = await API.binary('/api/tts', { text: text.slice(0, 400), language });
    if (!res.ok) throw new Error('TTS failed');
    const ctx = getTTSCtx();
    if (ctx.state === 'suspended') await ctx.resume();
    const audioBuffer = await ctx.decodeAudioData(await res.arrayBuffer());
    const src = ctx.createBufferSource();
    src.buffer = audioBuffer;
    src.connect(ctx.destination);
    ttsCurrentSrc = src;
    src.onended = () => {
      if (!src._stopped) { btn.textContent = '🔊 Play'; btn.classList.remove('playing'); }
      if (ttsCurrentSrc === src) ttsCurrentSrc = null;
    };
    src.start(0);
  } catch (e) {
    console.warn('Manual TTS error:', e);
    btn.textContent = '🔊 Play';
    btn.classList.remove('playing');
  }
}

/* ── Translation ────────────────────────────────────────────────────────────── */
async function translateMessage(btn) {
  const text = btn.getAttribute('data-text');
  if (!text) return;

  const body = btn.closest('.msg-body');
  const translationEl = body?.querySelector('.msg-translation');
  if (!translationEl) return;

  if (!translationEl.hidden) {
    translationEl.hidden = true;
    btn.textContent = '🌐 Translate';
    return;
  }

  if (translationCache.has(text)) {
    translationEl.textContent = translationCache.get(text);
    translationEl.hidden = false;
    btn.textContent = '🌐 Hide';
    return;
  }

  btn.textContent = '⏳…';
  btn.disabled = true;
  try {
    const data = await API.post('/api/conversation/translate', { text, language });
    if (!data?.translation) throw new Error();
    translationCache.set(text, data.translation);
    translationEl.textContent = data.translation;
    translationEl.hidden = false;
    btn.textContent = '🌐 Hide';
  } catch {
    translationEl.textContent = '⚠ Could not translate.';
    translationEl.hidden = false;
    btn.textContent = '🌐 Translate';
  } finally {
    btn.disabled = false;
  }
}

/* ── Voice (mic) button — toggles whether mic stream is active ──────────────── */
// In agent mode the mic is always streaming. The button just shows state.
// Tapping it mutes/unmutes the microphone capture.
let micMuted = false;

function toggleRecording() {
  if (!micStream || isAgentSpeaking) return; // block during agent's turn
  micMuted = !micMuted;
  micStream.getAudioTracks().forEach(t => { t.enabled = !micMuted; });
  const micBtn = document.getElementById('micBtn');
  const vs = document.getElementById('voiceStatus');
  if (micMuted) {
    micBtn.classList.remove('recording');
    vs.querySelector('span').textContent = 'Mic muted — tap to unmute';
  } else {
    micBtn.classList.add('recording');
    vs.querySelector('span').textContent = 'Listening… speak now';
  }
}

/* ── Timer ──────────────────────────────────────────────────────────────────── */
function startTimer() {
  sessionStartTime = Date.now();
  timerInterval = setInterval(() => {
    const elapsed = Math.floor((Date.now() - sessionStartTime) / 1000);
    const m = Math.floor(elapsed / 60);
    const s = elapsed % 60;
    const el = document.getElementById('convTimer');
    if (el) el.textContent = `${m}:${s.toString().padStart(2, '0')}`;
  }, 1000);
}

function getElapsedSecs() {
  if (!sessionStartTime) return 0;
  return Math.floor((Date.now() - sessionStartTime) / 1000);
}

/* ── End conversation ───────────────────────────────────────────────────────── */
async function endConversation() {
  if (!confirm('End this conversation and generate a learning summary?')) return;

  if (timerInterval) { clearInterval(timerInterval); timerInterval = null; }

  // Close WebSocket and stop mic
  if (ws) { ws.close(); ws = null; }
  stopMicrophone();
  if (pcmPlayer) pcmPlayer.clear();

  const durationSecs = getElapsedSecs();

  document.getElementById('sendBtn').disabled = true;
  document.getElementById('micBtn').disabled = true;
  document.querySelector('.btn-danger')?.setAttribute('disabled', '');

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
      transcript:    transcript,
      message_count: userMsgCount,
    });

    if (!data) return;
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
  if (ws) { ws.close(); ws = null; }
  stopMicrophone();
  window.location.href = '/dashboard.html';
}

/* ── HTML escaping ──────────────────────────────────────────────────────────── */
function escapeHtml(str) {
  return str
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/\n/g, '<br>');
}
function escapeAttr(str) {
  return str.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(function boot() {
  const overlay  = document.getElementById('startOverlay');
  const startBtn = document.getElementById('startBtn');
  overlay.hidden = false;

  startBtn.addEventListener('click', async () => {
    overlay.hidden = true;
    startTimer();

    // Init PCM player — AudioContext must be created inside user gesture on iOS
    pcmPlayer = new PCMPlayer(16000);
    pcmPlayer.resume();
    if (isMuted) pcmPlayer.setMuted(true);

    try {
      await connectToAgent();
    } catch (err) {
      console.error('Agent connection failed:', err);
      document.getElementById('loadingState')?.remove();
      appendMessage('assistant', '⚠ Could not connect to voice session: ' + (err.message || err));
    }
  }, { once: true });
})();
