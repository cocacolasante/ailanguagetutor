import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { ListeningSession, ListeningCompleteResult } from '@/types/api';

export const listeningApi = {
  startSession: (language: string, level: number) =>
    apiClient.post(ENDPOINTS.listeningSession, { language, level }).then((r) => r.data as ListeningSession),
  completeSession: (sessionId: string, answers: string[]) =>
    apiClient.post(ENDPOINTS.listeningComplete, { session_id: sessionId, answers }).then((r) => r.data as ListeningCompleteResult),
};
