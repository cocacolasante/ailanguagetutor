import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { WritingSession, WritingCompleteResult } from '@/types/api';

export const writingApi = {
  startSession: (language: string, level: number) =>
    apiClient.post(ENDPOINTS.writingSession, { language, level }).then((r) => r.data as WritingSession),
  completeSession: (sessionId: string) =>
    apiClient.post(ENDPOINTS.writingComplete, { session_id: sessionId }).then((r) => r.data as WritingCompleteResult),
};
