import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { UserStats, LeaderboardEntry, Badge, Mistake, ConversationRecord } from '@/types/api';

export const getStats = () =>
  apiClient.get<UserStats>(ENDPOINTS.userStats).then((r) => r.data);

export const getLeaderboard = () =>
  apiClient.get<LeaderboardEntry[]>(ENDPOINTS.leaderboard).then((r) => r.data);

export const getBadges = () =>
  apiClient.get<Badge[]>(ENDPOINTS.badges).then((r) => r.data);

export const getMistakes = () =>
  apiClient.get<{ mistakes: Mistake[] }>(ENDPOINTS.userMistakes).then((r) => r.data);

export const getConversationRecords = () =>
  apiClient.get<ConversationRecord[]>(ENDPOINTS.convRecords).then((r) => r.data);

export const getConversationRecord = (id: string) =>
  apiClient.get<ConversationRecord>(`${ENDPOINTS.convRecords}/${id}`).then((r) => r.data);
