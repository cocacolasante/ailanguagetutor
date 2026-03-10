import React, { useEffect } from 'react';
import { Stack } from 'expo-router';
import { router } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import * as Notifications from 'expo-notifications';

// Controls how foreground notifications are displayed
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});
import * as Linking from 'expo-linking';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';
import { useAuthStore } from '@/store/authStore';
import { getToken } from '@/utils/storage';
import { getMe } from '@/api/auth';

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 30_000 } },
});

function RootNav() {
  const { setAuth, clearAuth, setLoading, isLoading } = useAuthStore();

  useEffect(() => {
    async function hydrate() {
      try {
        const token = await getToken();
        if (!token) {
          clearAuth();
          router.replace('/(auth)/login');
          return;
        }
        const user = await getMe();
        setAuth(user, token);
        router.replace('/(app)');
      } catch {
        clearAuth();
        router.replace('/(auth)/login');
      } finally {
        setLoading(false);
        SplashScreen.hideAsync();
      }
    }
    hydrate();
  }, []);

  useEffect(() => {
    const sub = Linking.addEventListener('url', ({ url }) => {
      const parsed = Linking.parse(url);
      if (parsed.path === 'reset-password' && parsed.queryParams?.token) {
        router.push({
          pathname: '/(auth)/reset-password',
          params: { token: parsed.queryParams.token as string },
        });
      }
    });
    return () => sub.remove();
  }, []);

  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="(auth)" />
      <Stack.Screen name="(app)" />
      <Stack.Screen name="+not-found" />
    </Stack>
  );
}

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <SafeAreaProvider>
        <StatusBar style="light" />
        <RootNav />
      </SafeAreaProvider>
    </QueryClientProvider>
  );
}
