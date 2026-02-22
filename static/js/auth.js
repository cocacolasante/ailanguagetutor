/* ── Redirect if already logged in ─────────────────────────────────────────── */
if (localStorage.getItem('token')) {
  window.location.href = '/dashboard.html';
}

/* ── Tab switching ──────────────────────────────────────────────────────────── */
function switchTab(tab) {
  const loginPanel    = document.getElementById('panel-login');
  const registerPanel = document.getElementById('panel-register');
  const loginTab      = document.getElementById('tab-login');
  const registerTab   = document.getElementById('tab-register');
  hideAlert();

  if (tab === 'login') {
    loginPanel.style.display    = '';
    registerPanel.style.display = 'none';
    loginTab.classList.add('active');
    loginTab.setAttribute('aria-selected', 'true');
    registerTab.classList.remove('active');
    registerTab.setAttribute('aria-selected', 'false');
  } else {
    loginPanel.style.display    = 'none';
    registerPanel.style.display = '';
    loginTab.classList.remove('active');
    loginTab.setAttribute('aria-selected', 'false');
    registerTab.classList.add('active');
    registerTab.setAttribute('aria-selected', 'true');
  }
}

/* ── Alert helpers ──────────────────────────────────────────────────────────── */
function showAlert(msg) {
  const el = document.getElementById('authAlert');
  el.textContent = msg;
  el.classList.add('show');
}
function hideAlert() {
  document.getElementById('authAlert').classList.remove('show');
}

/* ── Set button loading state ───────────────────────────────────────────────── */
function setLoading(btn, loading) {
  btn.disabled = loading;
  btn.querySelector('span').textContent = loading ? 'Please wait…' : btn.dataset.label;
}

/* ── Login ──────────────────────────────────────────────────────────────────── */
const loginForm = document.getElementById('loginForm');
const loginBtn  = document.getElementById('loginBtn');
loginBtn.dataset.label = 'Sign In';

loginForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  hideAlert();

  const email    = document.getElementById('loginEmail').value.trim();
  const password = document.getElementById('loginPassword').value;

  if (!email || !password) { showAlert('Please fill in all fields.'); return; }

  setLoading(loginBtn, true);
  try {
    const data = await fetch('/api/auth/login', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ email, password }),
    }).then(async (res) => {
      const d = await res.json();
      if (!res.ok) throw new Error(d.error || 'Login failed');
      return d;
    });

    localStorage.setItem('token', data.token);
    localStorage.setItem('user',  JSON.stringify(data.user));
    window.location.href = '/dashboard.html';
  } catch (err) {
    showAlert(err.message);
  } finally {
    setLoading(loginBtn, false);
  }
});

/* ── Register ───────────────────────────────────────────────────────────────── */
const registerForm = document.getElementById('registerForm');
const registerBtn  = document.getElementById('registerBtn');
registerBtn.dataset.label = 'Create Account';

registerForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  hideAlert();

  const username = document.getElementById('regUsername').value.trim();
  const email    = document.getElementById('regEmail').value.trim();
  const password = document.getElementById('regPassword').value;

  if (!username || !email || !password) { showAlert('Please fill in all fields.'); return; }
  if (password.length < 8) { showAlert('Password must be at least 8 characters.'); return; }

  setLoading(registerBtn, true);
  try {
    const data = await fetch('/api/auth/register', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ email, username, password }),
    }).then(async (res) => {
      const d = await res.json();
      if (!res.ok) throw new Error(d.error || 'Registration failed');
      return d;
    });

    localStorage.setItem('token', data.token);
    localStorage.setItem('user',  JSON.stringify(data.user));
    window.location.href = '/dashboard.html';
  } catch (err) {
    showAlert(err.message);
  } finally {
    setLoading(registerBtn, false);
  }
});
