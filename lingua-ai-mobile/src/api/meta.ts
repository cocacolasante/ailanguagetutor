import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { Language, Topic, Personality } from '@/types/api';

export const getLanguages = () =>
  apiClient.get<Language[]>(ENDPOINTS.languages).then((r) => r.data);

export const getTopics = () =>
  apiClient.get<Topic[]>(ENDPOINTS.topics).then((r) => r.data);

export const getPersonalities = () =>
  apiClient.get<Personality[]>(ENDPOINTS.personalities).then((r) => r.data);
