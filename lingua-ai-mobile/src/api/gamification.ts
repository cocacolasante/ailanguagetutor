import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { UserStats, LeaderboardEntry, Badge } from '@/types/api';

export const getStats = () =>
  apiClient.get<UserStats>(ENDPOINTS.userStats).then((r) => r.data);

export const getLeaderboard = () =>
  apiClient.get<LeaderboardEntry[]>(ENDPOINTS.leaderboard).then((r) => r.data);

export const getBadges = () =>
  apiClient.get<Badge[]>(ENDPOINTS.badges).then((r) => r.data);
