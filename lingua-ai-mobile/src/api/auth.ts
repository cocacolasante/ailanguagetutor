import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { User } from '@/types/api';

export interface LoginRequest { email: string; password: string; }
export interface LoginResponse { token: string; user: User; status?: string; checkout_url?: string; }
export interface RegisterRequest { email: string; username: string; password: string; plan?: string; }
export interface RegisterResponse { message: string; }

export const login = (data: LoginRequest) =>
  apiClient.post<LoginResponse>(ENDPOINTS.login, data).then((r) => r.data);

export const register = (data: RegisterRequest) =>
  apiClient.post<RegisterResponse>(ENDPOINTS.register, data).then((r) => r.data);

export const logout = () =>
  apiClient.post(ENDPOINTS.logout).then((r) => r.data);

export const getMe = () =>
  apiClient.get<User>(ENDPOINTS.me).then((r) => r.data);

export const forgotPassword = (email: string) =>
  apiClient.post(ENDPOINTS.forgotPassword, { email }).then((r) => r.data);

export const resetPassword = (token: string, password: string) =>
  apiClient.post(ENDPOINTS.resetPassword, { token, password }).then((r) => r.data);

export const updatePreferences = (prefs: {
  pref_language?: string;
  pref_level?: number;
  pref_personality?: string;
}) => apiClient.patch(ENDPOINTS.preferences, prefs).then((r) => r.data);
