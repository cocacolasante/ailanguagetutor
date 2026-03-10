import { useEffect, useRef } from 'react';
import * as Notifications from 'expo-notifications';
import { router } from 'expo-router';
import { requestNotificationPermission, scheduleStreakReminder } from '@/utils/notifications';

/**
 * Call once from the home screen (or app layout) after login.
 * - Requests permission on first use
 * - Schedules a daily 8 PM streak reminder personalised with the current streak count
 * - Handles taps on notifications (navigates to the home tab)
 */
export function useNotifications(streak?: number) {
  const scheduledRef = useRef(false);

  // Permission + schedule
  useEffect(() => {
    if (streak === undefined) return; // wait until stats load
    if (scheduledRef.current) return; // already done this session

    (async () => {
      const granted = await requestNotificationPermission();
      if (!granted) return;
      await scheduleStreakReminder(streak);
      scheduledRef.current = true;
    })();
  }, [streak]);

  // Handle notification taps (deep-link to home)
  useEffect(() => {
    const sub = Notifications.addNotificationResponseReceivedListener(() => {
      router.replace('/(app)');
    });
    return () => sub.remove();
  }, []);
}
