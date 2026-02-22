/**
 * API client — shared across all pages.
 * Reads the JWT from localStorage and attaches it to every request.
 */
const API = (() => {
  const BASE = '';

  function token() {
    return localStorage.getItem('token') || '';
  }

  function headers(extra = {}) {
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token()}`,
      ...extra,
    };
  }

  async function request(method, path, body) {
    const opts = { method, headers: headers() };
    if (body !== undefined) opts.body = JSON.stringify(body);

    const res = await fetch(BASE + path, opts);
    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/';
      return;
    }
    return res;
  }

  async function json(method, path, body) {
    const res = await request(method, path, body);
    if (!res) return;
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  return {
    get:    (path)        => json('GET',    path),
    post:   (path, body)  => json('POST',   path, body),
    delete: (path)        => json('DELETE', path),

    /** Returns the raw Response for streaming endpoints. */
    stream: (path, body) =>
      fetch(BASE + path, {
        method:  'POST',
        headers: headers(),
        body:    JSON.stringify(body),
      }),

    /** Returns the raw Response for binary endpoints (TTS). */
    binary: (path, body) =>
      fetch(BASE + path, {
        method:  'POST',
        headers: headers(),
        body:    JSON.stringify(body),
      }),
  };
})();

/** Redirect to login if no token stored */
function requireAuth() {
  if (!localStorage.getItem('token')) {
    window.location.href = '/';
  }
}

function logout() {
  localStorage.removeItem('token');
  localStorage.removeItem('user');
  window.location.href = '/';
}
