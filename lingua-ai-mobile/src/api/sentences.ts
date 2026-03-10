import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { SentenceSession, SentenceCheckResult, SentenceCompleteResult } from '@/types/api';

export const sentencesApi = {
  startSession: (language: string, level: number) =>
    apiClient.post(ENDPOINTS.sentencesSession, { language, level }).then((r) => r.data as SentenceSession),
  checkAnswer: (sessionId: string, prompt: string, answer: string) =>
    apiClient.post(ENDPOINTS.sentencesCheck, { session_id: sessionId, prompt, answer }).then((r) => r.data as SentenceCheckResult),
  completeSession: (sessionId: string) =>
    apiClient.post(ENDPOINTS.sentencesComplete, { session_id: sessionId }).then((r) => r.data as SentenceCompleteResult),
};
