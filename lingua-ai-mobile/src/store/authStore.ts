import { create } from 'zustand';
import { User } from '@/types/api';
import { saveToken, deleteToken } from '@/utils/storage';

interface AuthState {
  user: User | null;
  token: string | null;
  isLoading: boolean;
  setAuth: (user: User, token: string) => void;
  clearAuth: () => void;
  setUser: (user: User) => void;
  setLoading: (loading: boolean) => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  isLoading: true,
  setAuth: (user, token) => {
    saveToken(token);
    set({ user, token, isLoading: false });
  },
  clearAuth: () => {
    deleteToken();
    set({ user: null, token: null, isLoading: false });
  },
  setUser: (user) => set({ user }),
  setLoading: (loading) => set({ isLoading: loading }),
}));
