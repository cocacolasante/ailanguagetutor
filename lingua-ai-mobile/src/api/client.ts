import axios from 'axios';
import { API_BASE } from '@/constants/api';
import { getToken, deleteToken } from '@/utils/storage';
import { router } from 'expo-router';

export const apiClient = axios.create({
  baseURL: API_BASE,
  headers: { 'Content-Type': 'application/json' },
});

apiClient.interceptors.request.use(async (config) => {
  const token = await getToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

apiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      await deleteToken();
      router.replace('/(auth)/login');
    }
    return Promise.reject(error);
  }
);
