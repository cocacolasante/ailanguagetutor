import { create } from 'zustand';

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

interface SessionState {
  sessionId: string | null;
  language: string;
  topic: string;
  topicName: string;
  level: number;
  personality: string;
  messages: ChatMessage[];
  startedAt: number | null;
  setSession: (s: {
    session_id: string;
    language: string;
    topic: string;
    topic_name: string;
    level: number;
    personality: string;
  }) => void;
  addMessage: (msg: ChatMessage) => void;
  updateLastAssistantMessage: (content: string) => void;
  clearSession: () => void;
}

export const useSessionStore = create<SessionState>((set) => ({
  sessionId: null,
  language: '',
  topic: '',
  topicName: '',
  level: 1,
  personality: '',
  messages: [],
  startedAt: null,
  setSession: (s) =>
    set({
      sessionId: s.session_id,
      language: s.language,
      topic: s.topic,
      topicName: s.topic_name,
      level: s.level,
      personality: s.personality,
      messages: [],
      startedAt: Date.now(),
    }),
  addMessage: (msg) =>
    set((state) => ({ messages: [...state.messages, msg] })),
  updateLastAssistantMessage: (content) =>
    set((state) => {
      const msgs = [...state.messages];
      const last = msgs[msgs.length - 1];
      if (last?.role === 'assistant') {
        msgs[msgs.length - 1] = { ...last, content };
      } else {
        msgs.push({ role: 'assistant', content });
      }
      return { messages: msgs };
    }),
  clearSession: () =>
    set({
      sessionId: null,
      language: '',
      topic: '',
      topicName: '',
      level: 1,
      personality: '',
      messages: [],
      startedAt: null,
    }),
}));
