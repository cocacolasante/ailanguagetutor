import React from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
} from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery } from '@tanstack/react-query';
import { FlatList } from 'react-native';
import { Colors } from '@/constants/colors';
import { StatsRow } from '@/components/dashboard/StatsRow';
import { AchievementChip } from '@/components/dashboard/AchievementChip';
import { Card } from '@/components/ui/Card';
import { ProgressBar } from '@/components/ui/ProgressBar';
import { Button } from '@/components/ui/Button';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getStats } from '@/api/gamification';
import { useAuthStore } from '@/store/authStore';
import { formatRelativeDate, levelLabel } from '@/utils/formatting';
import { ConversationRecord } from '@/types/api';

const LANGUAGE_FLAGS: Record<string, string> = { it: '🇮🇹', es: '🇪🇸', pt: '🇧🇷' };
const LANGUAGE_NAMES: Record<string, string> = { it: 'Italian', es: 'Spanish', pt: 'Portuguese' };

export default function DashboardScreen() {
  const user = useAuthStore((s) => s.user);
  const { data: stats, isLoading } = useQuery({ queryKey: ['stats'], queryFn: getStats });

  if (isLoading) return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;

  const languageCount = stats ? Object.keys(stats.language_fp ?? {}).length : 0;

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <View style={styles.greet}>
          <Text style={styles.greetText}>
            Welcome back, {user?.username ?? 'Learner'}! 👋
          </Text>
        </View>

        {stats && (
          <StatsRow
            streak={stats.streak}
            totalFP={stats.total_fp}
            languageCount={languageCount}
          />
        )}

        <Button
          title="Start Conversation"
          onPress={() => router.push('/(app)/conversation')}
          style={styles.ctaBtn}
        />

        {stats && Object.entries(stats.language_level ?? {}).length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Language Progress</Text>
            {Object.entries(stats.language_level).map(([lang, level]) => {
              const fp = stats.language_fp?.[lang] ?? 0;
              const nextLevelFP = level * 500;
              const progress = fp / nextLevelFP;
              return (
                <Card key={lang} style={styles.langCard}>
                  <View style={styles.langHeader}>
                    <Text style={styles.langFlag}>{LANGUAGE_FLAGS[lang] ?? '🌍'}</Text>
                    <View style={{ flex: 1 }}>
                      <Text style={styles.langName}>{LANGUAGE_NAMES[lang] ?? lang}</Text>
                      <Text style={styles.langLevel}>{levelLabel(level as number)}</Text>
                    </View>
                    <Text style={styles.langFP}>{fp} FP</Text>
                  </View>
                  <ProgressBar value={progress} style={{ marginTop: 8 }} />
                </Card>
              );
            })}
          </View>
        )}

        {stats && stats.achievements?.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Achievements</Text>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.achScroll}>
              {stats.achievements.map((id) => (
                <AchievementChip key={id} id={id} name={id} />
              ))}
            </ScrollView>
          </View>
        )}

        {stats && stats.recent_conversations?.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Recent Sessions</Text>
            <FlatList
              data={stats.recent_conversations}
              scrollEnabled={false}
              renderItem={({ item }: { item: ConversationRecord }) => (
                <Card style={styles.recentCard}>
                  <View style={styles.recentRow}>
                    <Text style={styles.recentFlag}>{LANGUAGE_FLAGS[item.language] ?? '🌍'}</Text>
                    <View style={{ flex: 1 }}>
                      <Text style={styles.recentTopic}>{item.topic_name}</Text>
                      <Text style={styles.recentMeta}>
                        {formatRelativeDate(item.created_at)} · +{item.fp_earned} FP
                      </Text>
                    </View>
                  </View>
                </Card>
              )}
              keyExtractor={(item) => item.id}
            />
          </View>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 16, paddingBottom: 32 },
  greet: { marginBottom: 4 },
  greetText: { color: Colors.textPrimary, fontSize: 18, fontWeight: '600' },
  ctaBtn: { marginTop: 4 },
  section: { gap: 12 },
  sectionTitle: { color: Colors.textPrimary, fontSize: 16, fontWeight: '700' },
  langCard: { gap: 4 },
  langHeader: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  langFlag: { fontSize: 28 },
  langName: { color: Colors.textPrimary, fontSize: 15, fontWeight: '600' },
  langLevel: { color: Colors.textSecondary, fontSize: 12 },
  langFP: { color: Colors.accent, fontSize: 13, fontWeight: '600' },
  achScroll: { marginTop: 4 },
  recentCard: { marginBottom: 8 },
  recentRow: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  recentFlag: { fontSize: 24 },
  recentTopic: { color: Colors.textPrimary, fontSize: 14, fontWeight: '600' },
  recentMeta: { color: Colors.textSecondary, fontSize: 12, marginTop: 2 },
});
