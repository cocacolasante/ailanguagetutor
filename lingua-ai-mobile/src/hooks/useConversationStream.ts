import EventSource from 'react-native-sse';
import { API_BASE, ENDPOINTS } from '@/constants/api';

export function useConversationStream() {
  const stream = (
    sessionId: string,
    message: string,
    token: string,
    onToken: (t: string) => void,
    onDone: () => void
  ): (() => void) => {
    const es = new EventSource(`${API_BASE}${ENDPOINTS.convMessage}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        session_id: sessionId,
        message,
        greet: message === '',
      }),
    });

    es.addEventListener('message', (e) => {
      if (!e.data) return;
      try {
        const parsed = JSON.parse(e.data);
        if (parsed.done) {
          onDone();
          es.close();
          return;
        }
        if (parsed.content) onToken(parsed.content);
      } catch {
        // ignore parse errors
      }
    });

    es.addEventListener('error', () => {
      es.close();
    });

    return () => es.close();
  };

  return { stream };
}
