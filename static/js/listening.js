requireAuth();

/* ── URL params ─────────────────────────────────────────────────────────────── */
const params      = new URLSearchParams(window.location.search);
const language    = params.get('language')    || 'it';
const level       = parseInt(params.get('level') || '1', 10);
const topic       = params.get('topic')       || 'general';
const topicName   = params.get('topicName')   || 'General';
const personality = params.get('personality') || 'professor';

/* ── State ──────────────────────────────────────────────────────────────────── */
let story        = null;   // Story object from API
let speed        = 1.0;    // TTS speed for this level
let currentIdx   = 0;      // current segment index
let results      = [];     // {question_index, correct}[]
let audioCtx     = null;
let currentSrc   = null;   // currently playing AudioBufferSourceNode

/* ── Language helpers ─────────────────────────────────────────────────────────── */
const LANG_NAMES = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };
const LANG_FLAGS = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' };

/* ── Boot ───────────────────────────────────────────────────────────────────── */
(async function init() {
  await loadSession();
})();

/* ── Load session ───────────────────────────────────────────────────────────── */
async function loadSession() {
  try {
    const data = await API.post('/api/listening/session', { language, level, topic, personality });
    story = data.story;
    speed = data.speed || 1.0;

    if (!story || !story.segments || story.segments.length === 0) {
      showError('No story returned. Please try again.');
      return;
    }

    const flag = LANG_FLAGS[language] || '🌐';
    document.getElementById('storyTitle').textContent  = story.title || 'Listening Comprehension';
    document.getElementById('headerSub').textContent   =
      `${flag} ${LANG_NAMES[language] || language} · Level ${level} · ${story.segments.length} segments`;

    document.getElementById('loadingState').classList.add('hidden');
    document.getElementById('storyContainer').classList.remove('hidden');

    renderSegment(currentIdx);
    setTimeout(() => playSegment(), 600);
  } catch (err) {
    showError('Failed to load story. ' + (err.message || ''));
  }
}

/* ── Render segment ─────────────────────────────────────────────────────────── */
function renderSegment(idx) {
  const seg   = story.segments[idx];
  const total = story.segments.length;

  // Progress
  const pct = (idx / total) * 100;
  document.getElementById('progressFill').style.width  = pct + '%';
  document.getElementById('progressLabel').textContent = `Segment ${idx + 1} of ${total}`;

  // Segment text
  document.getElementById('segmentText').textContent = seg.text;

  // Reset question + feedback
  document.getElementById('questionCard').classList.add('hidden');
  document.getElementById('feedbackZone').classList.add('hidden');
  document.getElementById('nextBtn').classList.add('hidden');
  document.getElementById('resultsBtn').classList.add('hidden');

  // Re-enable play button
  const playBtn = document.getElementById('playBtn');
  playBtn.disabled = false;
  playBtn.textContent = '🔊 Play';
}

/* ── TTS playback ───────────────────────────────────────────────────────────── */
function getAudioCtx() {
  if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  return audioCtx;
}

async function playSegment() {
  const seg = story.segments[currentIdx];
  if (!seg) return;
  stopAudio();
  const btn = document.getElementById('playBtn');
  btn.disabled = true;
  btn.textContent = '⏳';
  try {
    const res = await API.binary('/api/tts', { text: seg.text, language, personality, speed });
    if (!res.ok) throw new Error('TTS failed');
    const ctx = getAudioCtx();
    if (ctx.state === 'suspended') await ctx.resume();
    const buf = await ctx.decodeAudioData(await res.arrayBuffer());
    const src = ctx.createBufferSource();
    src.buffer = buf;
    src.connect(ctx.destination);
    src.start(0);
    currentSrc = src;
    src.onended = () => {
      currentSrc = null;
      btn.disabled = false;
      btn.textContent = '🔊 Replay';
      showQuestion();
    };
  } catch {
    btn.disabled = false;
    btn.textContent = '🔊 Play';
    showQuestion(); // show question even if TTS fails
  }
}

function stopAudio() {
  if (currentSrc) {
    try { currentSrc.stop(); } catch {}
    currentSrc = null;
  }
}

/* ── Show question ──────────────────────────────────────────────────────────── */
function showQuestion() {
  const seg = story.segments[currentIdx];
  if (!seg) return;
  const q = seg.question;

  document.getElementById('questionText').textContent = q.question;
  document.getElementById('questionCard').classList.remove('hidden');

  const container = document.getElementById('answerBtns');
  container.innerHTML = '';

  if (q.type === 'multiple_choice' && q.options) {
    q.options.forEach((opt, i) => {
      const btn = document.createElement('button');
      btn.className = 'btn btn-ghost';
      btn.style.cssText = 'min-width:120px;flex:1 1 40%';
      btn.textContent = opt;
      btn.onclick = () => submitAnswer(i);
      container.appendChild(btn);
    });
  } else if (q.type === 'true_false') {
    ['True', 'False'].forEach(label => {
      const btn = document.createElement('button');
      btn.className = 'btn btn-ghost';
      btn.style.cssText = 'min-width:120px;flex:1 0 40%';
      btn.textContent = label;
      btn.onclick = () => submitAnswer(label.toLowerCase());
      container.appendChild(btn);
    });
  } else if (q.type === 'yes_no') {
    ['Yes', 'No'].forEach(label => {
      const btn = document.createElement('button');
      btn.className = 'btn btn-ghost';
      btn.style.cssText = 'min-width:120px;flex:1 0 40%';
      btn.textContent = label;
      btn.onclick = () => submitAnswer(label.toLowerCase());
      container.appendChild(btn);
    });
  }
}

/* ── Submit answer ──────────────────────────────────────────────────────────── */
function submitAnswer(userAnswer) {
  const seg = story.segments[currentIdx];
  const q   = seg.question;

  // Disable all answer buttons
  document.querySelectorAll('#answerBtns button').forEach(b => b.disabled = true);

  // Check correctness
  let correct = false;
  if (q.type === 'multiple_choice') {
    correct = (userAnswer === q.answer);
  } else {
    correct = (String(userAnswer).toLowerCase() === String(q.answer).toLowerCase());
  }

  results.push({ question_index: currentIdx, correct });

  // Show feedback
  const fbStatus = document.getElementById('feedbackStatus');
  const fbExpl   = document.getElementById('explanationText');
  fbStatus.textContent = correct ? '✓ Correct!' : '✗ Incorrect';
  fbStatus.style.color = correct ? '#10b981' : '#f87171';
  fbExpl.textContent   = q.explanation || '';
  document.getElementById('feedbackZone').classList.remove('hidden');

  const isLast = (currentIdx >= story.segments.length - 1);
  if (isLast) {
    document.getElementById('resultsBtn').classList.remove('hidden');
  } else {
    document.getElementById('nextBtn').classList.remove('hidden');
  }
}

/* ── Next segment ───────────────────────────────────────────────────────────── */
function nextSegment() {
  stopAudio();
  currentIdx++;
  renderSegment(currentIdx);
  setTimeout(() => playSegment(), 400);
}

/* ── Results ────────────────────────────────────────────────────────────────── */
async function showResults() {
  stopAudio();

  let fp = 0;
  let correctCount = 0;
  const totalCount = results.length;

  // Call complete endpoint
  try {
    const data = await API.post('/api/listening/complete', {
      language,
      level,
      topic,
      topic_name: topicName,
      personality,
      results,
    });
    fp           = data.fp_earned || 0;
    correctCount = data.correct_count !== undefined ? data.correct_count : results.filter(r => r.correct).length;
  } catch {
    correctCount = results.filter(r => r.correct).length;
    fp = Math.max(20, correctCount * 15) + (correctCount === totalCount && totalCount > 0 ? 20 : 0);
  }

  document.getElementById('statScore').textContent = `${correctCount} / ${totalCount}`;
  document.getElementById('statFP').textContent    = `+${fp} FP`;

  document.getElementById('storyContainer').classList.add('hidden');
  document.getElementById('resultsScreen').classList.remove('hidden');
}

/* ── Listen again ───────────────────────────────────────────────────────────── */
function listenAgain() {
  currentIdx = 0;
  results    = [];
  stopAudio();
  document.getElementById('resultsScreen').classList.add('hidden');
  document.getElementById('storyContainer').classList.remove('hidden');
  renderSegment(0);
  setTimeout(() => playSegment(), 400);
}

/* ── Error helper ───────────────────────────────────────────────────────────── */
function showError(msg) {
  document.getElementById('loadingState').innerHTML =
    `<p style="color:#f87171">${msg}</p>
     <a href="/dashboard.html" class="btn btn-ghost" style="margin-top:16px">← Dashboard</a>`;
}
