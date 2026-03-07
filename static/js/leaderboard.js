// Leaderboard is public — no auth required but we check for current user to highlight
const currentUser = (() => {
  try {
    const token = localStorage.getItem('token');
    const payload = token ? JSON.parse(atob(token.split('.')[1])) : null;
    return payload?.sub || null;
  } catch (_) { return null; }
})();

const currentUsername = (() => {
  try {
    const raw = localStorage.getItem('user');
    if (!raw) return null;
    return JSON.parse(raw).username || null;
  } catch (_) { return null; }
})();

async function loadLeaderboard() {
  try {
    const data = await fetch('/api/leaderboard').then(r => r.json());
    renderLeaderboard(data.leaderboard || []);
  } catch (err) {
    document.getElementById('leaderboardContainer').innerHTML =
      '<p style="color:var(--text-2);text-align:center;padding:32px">Failed to load leaderboard.</p>';
  }
}

function renderLeaderboard(entries) {
  const container = document.getElementById('leaderboardContainer');

  if (entries.length === 0) {
    container.innerHTML = `
      <div class="leaderboard-empty">
        <p>No entries yet. Complete a conversation to get on the board!</p>
        <a href="/dashboard.html" class="btn btn-primary">Start Practicing →</a>
      </div>`;
    return;
  }

  const medals = ['🥇', '🥈', '🥉'];

  const rows = entries.map(entry => {
    const isMe = entry.username === currentUsername;
    const rank = entry.rank;
    const medal = rank <= 3 ? medals[rank - 1] : '';
    const rankDisplay = medal ? `<span class="lb-medal">${medal}</span>` : `<span class="lb-rank">#${rank}</span>`;

    return `
      <div class="lb-row${isMe ? ' lb-row-me' : ''}">
        <div class="lb-rank-cell">${rankDisplay}</div>
        <div class="lb-name-cell">
          <span class="lb-avatar">${entry.username.charAt(0).toUpperCase()}</span>
          <span class="lb-username">${escapeHtml(entry.username)}${isMe ? ' <span class="lb-you">you</span>' : ''}</span>
        </div>
        <div class="lb-fp-cell">
          <span class="lb-fp">${entry.total_fp.toLocaleString()} FP</span>
        </div>
        <div class="lb-streak-cell">
          ${entry.streak > 0 ? `🔥 ${entry.streak}d` : '—'}
        </div>
      </div>`;
  }).join('');

  container.innerHTML = `
    <div class="lb-header-row">
      <div class="lb-rank-cell">Rank</div>
      <div class="lb-name-cell">Learner</div>
      <div class="lb-fp-cell">Fluency Points</div>
      <div class="lb-streak-cell">Streak</div>
    </div>
    ${rows}
  `;
}

function escapeHtml(str) {
  return (str || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

loadLeaderboard();
