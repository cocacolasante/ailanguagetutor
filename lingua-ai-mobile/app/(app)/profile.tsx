import React, { useEffect } from 'react';
import {
  View, Text, StyleSheet, ScrollView, Alert,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import * as Linking from 'expo-linking';
import { useQuery } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { AchievementChip } from '@/components/dashboard/AchievementChip';
import { useAuthStore } from '@/store/authStore';
import { useSubscription } from '@/hooks/useSubscription';
import { getBadges } from '@/api/gamification';
import { getStats } from '@/api/gamification';
import { createPortalSession, createCheckout } from '@/api/billing';

const STATUS_COLORS: Record<string, string> = {
  active: Colors.success,
  free: Colors.success,
  trialing: Colors.warning,
  past_due: Colors.warning,
  cancelled: Colors.error,
  suspended: Colors.error,
};

export default function ProfileScreen() {
  const user = useAuthStore((s) => s.user);
  const { status, trialEndsAt, refetch: refetchSub } = useSubscription();

  const { data: badges } = useQuery({ queryKey: ['badges'], queryFn: getBadges });
  const { data: stats } = useQuery({ queryKey: ['stats'], queryFn: getStats });

  // Re-fetch billing status when screen comes into focus (user returning from Stripe)
  useEffect(() => {
    refetchSub();
  }, []);

  const handlePortal = async () => {
    try {
      const { url } = await createPortalSession();
      await Linking.openURL(url);
    } catch {
      Alert.alert('Error', 'Unable to open billing portal.');
    }
  };

  const handleUpgrade = async () => {
    try {
      const { url } = await createCheckout('immediate');
      await Linking.openURL(url);
    } catch {
      Alert.alert('Error', 'Unable to open checkout.');
    }
  };

  const earnedAchievements = new Set(stats?.achievements ?? []);

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <View style={styles.userSection}>
          <View style={styles.avatar}>
            <Text style={styles.avatarText}>{user?.username?.[0]?.toUpperCase() ?? '?'}</Text>
          </View>
          <View>
            <Text style={styles.username}>{user?.username}</Text>
            <Text style={styles.email}>{user?.email}</Text>
          </View>
        </View>

        <Card style={styles.subCard}>
          <Text style={styles.subLabel}>Subscription</Text>
          <View style={styles.subRow}>
            <Badge label={status || 'Unknown'} color={STATUS_COLORS[status] ?? Colors.textSecondary} />
            {trialEndsAt && (
              <Text style={styles.trialEnd}>
                Ends {new Date(trialEndsAt).toLocaleDateString()}
              </Text>
            )}
          </View>
          {(status === 'active' || status === 'free' || status === 'trialing' || status === 'past_due') && (
            <Button title="Manage Billing" onPress={handlePortal} variant="secondary" style={styles.subBtn} />
          )}
          {(status === 'cancelled' || status === '' || status === 'suspended') && (
            <Button title="Upgrade Now" onPress={handleUpgrade} style={styles.subBtn} />
          )}
        </Card>

        {badges && badges.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Achievements ({stats?.achievements?.length ?? 0}/{badges.length})</Text>
            <View style={styles.badgeGrid}>
              {badges.map((b) => (
                <AchievementChip
                  key={b.id}
                  id={b.id}
                  name={b.name}
                  icon={b.icon}
                  earned={earnedAchievements.has(b.id)}
                />
              ))}
            </View>
          </View>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 20, paddingBottom: 32 },
  userSection: { flexDirection: 'row', alignItems: 'center', gap: 14 },
  avatar: {
    width: 60,
    height: 60,
    borderRadius: 30,
    backgroundColor: Colors.accent,
    alignItems: 'center',
    justifyContent: 'center',
  },
  avatarText: { color: '#fff', fontSize: 24, fontWeight: '700' },
  username: { color: Colors.textPrimary, fontSize: 18, fontWeight: '700' },
  email: { color: Colors.textSecondary, fontSize: 14, marginTop: 2 },
  subCard: { gap: 10 },
  subLabel: { color: Colors.textSecondary, fontSize: 13, fontWeight: '600', textTransform: 'uppercase' },
  subRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  trialEnd: { color: Colors.textSecondary, fontSize: 12 },
  subBtn: { marginTop: 4 },
  section: { gap: 12 },
  sectionTitle: { color: Colors.textPrimary, fontSize: 16, fontWeight: '700' },
  badgeGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
});
