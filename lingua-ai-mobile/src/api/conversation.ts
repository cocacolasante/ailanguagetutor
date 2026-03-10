import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { SessionStart, EndConversationResponse, ConversationRecord } from '@/types/api';

export interface StartConversationRequest {
  language: string;
  topic: string;
  level: number;
  personality: string;
}

export const startConversation = (data: StartConversationRequest) =>
  apiClient.post<SessionStart>(ENDPOINTS.convStart, data).then((r) => r.data);

export const endConversation = (session_id: string) =>
  apiClient.post<EndConversationResponse>(ENDPOINTS.convEnd, { session_id }).then((r) => r.data);

export const translateMessage = (session_id: string, text: string) =>
  apiClient.post<{ translation: string }>(ENDPOINTS.convTranslate, { session_id, text }).then((r) => r.data);

export const getConversationRecords = () =>
  apiClient.get<ConversationRecord[]>(ENDPOINTS.convRecords).then((r) => r.data);

export const getConversationRecord = (id: string) =>
  apiClient.get<ConversationRecord>(`${ENDPOINTS.convRecords}/${id}`).then((r) => r.data);
