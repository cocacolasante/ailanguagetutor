const BASE = process.env.EXPO_PUBLIC_API_BASE_URL ?? 'http://localhost:8080';
export const API_BASE = BASE;

export const ENDPOINTS = {
  login: '/api/auth/login',
  register: '/api/auth/register',
  logout: '/api/auth/logout',
  me: '/api/auth/me',
  forgotPassword: '/api/auth/forgot-password',
  resetPassword: '/api/auth/reset-password',
  preferences: '/api/user/preferences',
  convStart: '/api/conversation/start',
  convMessage: '/api/conversation/message',
  convEnd: '/api/conversation/end',
  convTranslate: '/api/conversation/translate',
  convRecords: '/api/conversation/records',
  userStats: '/api/user/stats',
  leaderboard: '/api/leaderboard',
  badges: '/api/badges',
  billingStatus: '/api/billing/status',
  billingCheckout: '/api/billing/checkout',
  billingPortal: '/api/billing/portal',
  tts: '/api/tts',
  languages: '/api/languages',
  topics: '/api/topics',
  personalities: '/api/personalities',
};
