import * as Notifications from 'expo-notifications';
import { Platform } from 'react-native';

const STREAK_NOTIF_ID = 'daily-streak-reminder';
const REMINDER_HOUR = 20; // 8:00 PM local time
const REMINDER_MINUTE = 0;

export async function requestNotificationPermission(): Promise<boolean> {
  if (Platform.OS === 'web') return false;

  const { status: existing } = await Notifications.getPermissionsAsync();
  if (existing === 'granted') return true;

  const { status } = await Notifications.requestPermissionsAsync();
  return status === 'granted';
}

export async function scheduleStreakReminder(streak: number): Promise<void> {
  // Cancel existing so we don't duplicate
  await Notifications.cancelScheduledNotificationAsync(STREAK_NOTIF_ID).catch(() => {});

  const title = streak > 1
    ? `🔥 ${streak}-day streak — don't break it!`
    : '📚 Time to practice!';

  const body = streak > 1
    ? `Keep your ${streak}-day streak alive. Even 5 minutes counts.`
    : "You haven't practiced today. Start a quick session to build your streak!";

  await Notifications.scheduleNotificationAsync({
    identifier: STREAK_NOTIF_ID,
    content: { title, body, sound: true },
    trigger: {
      type: Notifications.SchedulableTriggerInputTypes.DAILY,
      hour: REMINDER_HOUR,
      minute: REMINDER_MINUTE,
    },
  });
}

/**
 * Call after completing any practice session.
 * Cancels today's reminder so the user isn't nagged after practicing.
 * The hook will reschedule it on next app open.
 */
export async function cancelStreakReminder(): Promise<void> {
  await Notifications.cancelScheduledNotificationAsync(STREAK_NOTIF_ID).catch(() => {});
}
