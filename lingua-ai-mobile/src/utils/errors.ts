import axios from 'axios';

export function isPracticeSessionExpired(error: unknown): boolean {
  return axios.isAxiosError(error) && error.response?.status === 404;
}

export function extractErrorMessage(err: unknown): string {
  if (err && typeof err === 'object') {
    const e = err as Record<string, unknown>;
    if (e.response && typeof e.response === 'object') {
      const r = e.response as Record<string, unknown>;
      if (r.data && typeof r.data === 'object') {
        const d = r.data as Record<string, unknown>;
        if (typeof d.error === 'string') return d.error;
        if (typeof d.message === 'string') return d.message;
      }
    }
    if (typeof e.message === 'string') return e.message;
  }
  return 'An unexpected error occurred.';
}
