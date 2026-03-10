import { useAuthStore } from '@/store/authStore';

export function useAuth() {
  const { user, token, isLoading, setAuth, clearAuth, setUser } = useAuthStore();
  return { user, token, isLoading, setAuth, clearAuth, setUser, isAuthenticated: !!token };
}
