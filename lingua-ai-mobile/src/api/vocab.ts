import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { VocabSession, VocabCheckResult, VocabCompleteResult } from '@/types/api';

export const vocabApi = {
  startSession: (language: string, level: number) =>
    apiClient.post(ENDPOINTS.vocabSession, { language, level }).then((r) => r.data as VocabSession),
  checkAnswer: (sessionId: string, word: string, answer: string) =>
    apiClient.post(ENDPOINTS.vocabCheck, { session_id: sessionId, word, answer }).then((r) => r.data as VocabCheckResult),
  recordResult: (sessionId: string, word: string, correct: boolean) =>
    apiClient.post(ENDPOINTS.vocabWordResult, { session_id: sessionId, word, correct }).then((r) => r.data),
  completeSession: (sessionId: string) =>
    apiClient.post(ENDPOINTS.vocabComplete, { session_id: sessionId }).then((r) => r.data as VocabCompleteResult),
};
