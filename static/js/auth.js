/* ── Redirect if already logged in ─────────────────────────────────────────── */
if (localStorage.getItem('token')) {
  window.location.href = '/dashboard.html';
}

/* ── Handle query params on page load ───────────────────────────────────────── */
(function handleQueryParams() {
  const p = new URLSearchParams(location.search);
  if (p.get('checkout') === 'cancelled') {
    setTimeout(() => showAlert('Checkout was cancelled. You can try again below.'), 100);
    switchTab('register');
  } else if (p.get('verified') === 'true') {
    setTimeout(() => showAlert('Email verified! Sign in to continue setting up your subscription.', 'success'), 100);
    switchTab('login');
  } else if (p.get('error') === 'invalid_token') {
    setTimeout(() => showAlert('This verification link is invalid or has already been used. Please register again or sign in.'), 100);
  } else if (p.get('error') === 'server_error') {
    setTimeout(() => showAlert('Something went wrong. Please try signing in.'), 100);
    switchTab('login');
  }
})();

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
function showAlert(msg, type) {
  const el = document.getElementById('authAlert');
  el.textContent = msg;
  el.className = 'alert ' + (type === 'success' ? 'alert-success' : 'alert-error') + ' show';
}
function hideAlert() {
  const el = document.getElementById('authAlert');
  el.classList.remove('show');
}

/* ── Set button loading state ───────────────────────────────────────────────── */
function setLoading(btn, loading) {
  btn.disabled = loading;
  btn.querySelector('span').textContent = loading ? 'Please wait…' : btn.dataset.label;
}

/* ── Plan selection ─────────────────────────────────────────────────────────── */
document.querySelectorAll('.plan-option').forEach(label => {
  label.addEventListener('click', () => {
    const radio = label.querySelector('input[type="radio"]');
    if (radio) radio.checked = true;
    document.querySelectorAll('.plan-option').forEach(el => el.classList.remove('selected'));
    label.classList.add('selected');
  });
});

/* ── Login ──────────────────────────────────────────────────────────────────── */
const loginForm = document.getElementById('loginForm');
const loginBtn  = document.getElementById('loginBtn');
loginBtn.dataset.label = loginBtn.querySelector('span').textContent;

loginForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  hideAlert();

  const email    = document.getElementById('loginEmail').value.trim();
  const password = document.getElementById('loginPassword').value;

  if (!email || !password) { showAlert('Please fill in all fields.'); return; }

  setLoading(loginBtn, true);
  try {
    const res  = await fetch('/api/auth/login', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ email, password }),
    });
    const data = await res.json();

    if (!res.ok) {
      if (data.status === 'email_unverified') {
        showAlert(data.error || 'Please verify your email before signing in.');
        return;
      }
      // If subscription is missing and we have a saved Stripe session_id, verify
      // the checkout directly (webhook may not have fired yet in local dev).
      if (data.status === '') {
        const returnUrl  = sessionStorage.getItem('authReturnUrl') || '';
        const returnSearch = returnUrl.includes('?') ? returnUrl.split('?')[1] : '';
        const rp = new URLSearchParams(returnSearch);
        const savedSessionId = rp.get('session_id');
        if (savedSessionId && rp.get('checkout') === 'success') {
          try {
            const vr = await fetch('/api/billing/verify-checkout?session_id=' + savedSessionId);
            if (vr.ok) {
              // Subscription now set — retry login automatically
              const rr  = await fetch('/api/auth/login', {
                method:  'POST',
                headers: { 'Content-Type': 'application/json' },
                body:    JSON.stringify({ email, password }),
              });
              const rd = await rr.json();
              if (rr.ok) {
                localStorage.setItem('token', rd.token);
                if (rd.user) localStorage.setItem('user', JSON.stringify(rd.user));
                sessionStorage.removeItem('authReturnUrl');
                window.location.href = '/dashboard.html';
                return;
              }
            }
          } catch (_) {}
        }
      }
      if (data.checkout_url) {
        window.location.href = data.checkout_url;
      } else {
        showAlert(data.error || 'Login failed');
      }
      return;
    }

    localStorage.setItem('token', data.token);
    if (data.user) localStorage.setItem('user', JSON.stringify(data.user));
    // Restore any URL saved before the login redirect (e.g. Stripe return params)
    const returnUrl = sessionStorage.getItem('authReturnUrl');
    sessionStorage.removeItem('authReturnUrl');
    window.location.href = returnUrl || '/dashboard.html';
  } catch (err) {
    showAlert(err.message);
  } finally {
    setLoading(loginBtn, false);
  }
});

/* ── Register ───────────────────────────────────────────────────────────────── */
const registerForm = document.getElementById('registerForm');
const registerBtn  = document.getElementById('registerBtn');
registerBtn.dataset.label = registerBtn.querySelector('span').textContent;

registerForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  hideAlert();

  const username = document.getElementById('regUsername').value.trim();
  const email    = document.getElementById('regEmail').value.trim();
  const password = document.getElementById('regPassword').value;
  const plan     = document.querySelector('input[name="plan"]:checked')?.value || 'trial';

  if (!username || !email || !password) { showAlert('Please fill in all fields.'); return; }
  if (password.length < 8) { showAlert('Password must be at least 8 characters.'); return; }

  setLoading(registerBtn, true);
  try {
    const res  = await fetch('/api/auth/register', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ email, username, password, plan }),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Registration failed');

    if (data.token) {
      // Admin — immediate access, no email verification needed
      localStorage.setItem('token', data.token);
      localStorage.setItem('user',  JSON.stringify(data.user));
      window.location.href = '/dashboard.html';
    } else if (data.message) {
      // Email verification sent — show the check-email panel
      showEmailSentPanel(email);
    } else {
      showAlert(data.message || 'Account created.');
    }
  } catch (err) {
    showAlert(err.message);
  } finally {
    setLoading(registerBtn, false);
  }
});

function showEmailSentPanel(email) {
  document.getElementById('panel-login').style.display    = 'none';
  document.getElementById('panel-register').style.display = 'none';
  document.getElementById('panel-email-sent').style.display = '';
  document.getElementById('emailSentAddr').textContent = email;
  // Hide tabs while on the email-sent panel
  document.querySelector('.auth-tabs').style.display = 'none';
  hideAlert();
}
