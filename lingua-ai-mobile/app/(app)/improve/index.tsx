import React, { useState } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity, FlatList, RefreshControl,
} from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getStats, getMistakes, getConversationRecords } from '@/api/gamification';
import { queryKeys } from '@/constants/api';
import { ConversationRecord, Mistake } from '@/types/api';

export default function ImproveScreen() {
  const queryClient = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  const statsQuery = useQuery({ queryKey: queryKeys.userStats, queryFn: getStats });
  const mistakesQuery = useQuery({ queryKey: queryKeys.userMistakes, queryFn: getMistakes });
  const recordsQuery = useQuery({ queryKey: queryKeys.records, queryFn: getConversationRecords });

  const handleRefresh = async () => {
    setRefreshing(true);
    await Promise.all([
      statsQuery.refetch(),
      mistakesQuery.refetch(),
      recordsQuery.refetch(),
    ]);
    setRefreshing(false);
  };

  const isLoading = statsQuery.isLoading || mistakesQuery.isLoading || recordsQuery.isLoading;
  if (isLoading) return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;

  const stats = statsQuery.data;
  const mistakes = mistakesQuery.data?.mistakes ?? [];
  const records = (recordsQuery.data ?? []).slice(0, 5);

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView
        contentContainerStyle={styles.content}
        showsVerticalScrollIndicator={false}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={handleRefresh} tintColor={Colors.accent} />
        }
      >
        <Text style={styles.title}>Improve</Text>
        <Text style={styles.subtitle}>Personalized insights to accelerate learning</Text>

        {/* Weak Areas */}
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Weak Areas</Text>
          {stats?.weak_areas && stats.weak_areas.length > 0 ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              {stats.weak_areas.map((area) => (
                <TouchableOpacity
                  key={area}
                  onPress={() => router.push('/(app)/practice' as never)}
                >
                  <Card style={styles.chip}>
                    <Text style={styles.chipText}>{area}</Text>
                  </Card>
                </TouchableOpacity>
              ))}
            </ScrollView>
          ) : (
            <Text style={styles.emptyText}>No weak areas identified yet. Keep practicing!</Text>
          )}
        </View>

        {/* Common Mistakes */}
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Common Mistakes</Text>
          {mistakes.length > 0 ? (
            mistakes.map((m: Mistake, i: number) => (
              <Card key={i} style={styles.mistakeCard}>
                <View style={styles.mistakeHeader}>
                  <Text style={styles.mistakeType}>{m.type}</Text>
                  <View style={styles.countBadge}>
                    <Text style={styles.countText}>{m.count}x</Text>
                  </View>
                </View>
                <Text style={styles.mistakeDesc}>{m.description}</Text>
                {m.example ? (
                  <Text style={styles.mistakeExample}>e.g. "{m.example}"</Text>
                ) : null}
              </Card>
            ))
          ) : (
            <Text style={styles.emptyText}>No mistakes recorded yet.</Text>
          )}
        </View>

        {/* Weak Vocabulary */}
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Weak Vocabulary</Text>
          {stats?.weak_vocab && stats.weak_vocab.length > 0 ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              {stats.weak_vocab.map((word) => (
                <Card key={word} style={styles.vocabChip}>
                  <Text style={styles.vocabText}>{word}</Text>
                </Card>
              ))}
            </ScrollView>
          ) : (
            <Text style={styles.emptyText}>No weak vocabulary identified yet.</Text>
          )}
        </View>

        {/* Next Suggestions */}
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Next Suggestions</Text>
          {stats?.next_suggestions && stats.next_suggestions.length > 0 ? (
            stats.next_suggestions.slice(0, 3).map((suggestion, i) => (
              <Card key={i} style={styles.suggestionCard}>
                <Text style={styles.suggestionText}>{suggestion}</Text>
                <TouchableOpacity
                  style={styles.startBtn}
                  onPress={() => router.push('/(app)/conversation' as never)}
                >
                  <Text style={styles.startBtnText}>Start →</Text>
                </TouchableOpacity>
              </Card>
            ))
          ) : (
            <Text style={styles.emptyText}>Complete more sessions to get suggestions.</Text>
          )}
        </View>

        {/* Recent Sessions */}
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Recent Sessions</Text>
          {records.length > 0 ? (
            records.map((record: ConversationRecord) => (
              <TouchableOpacity
                key={record.id}
                onPress={() => router.push(`/(app)/improve/${record.id}` as never)}
              >
                <Card style={styles.recordCard}>
                  <View style={styles.recordHeader}>
                    <Text style={styles.recordLanguage}>
                      {record.language.toUpperCase()} · Level {record.level}
                    </Text>
                    <Text style={styles.recordFP}>+{record.fp_earned} FP</Text>
                  </View>
                  <Text style={styles.recordTopic}>{record.topic_name}</Text>
                  <Text style={styles.recordDate}>
                    {new Date(record.created_at).toLocaleDateString()}
                  </Text>
                </Card>
              </TouchableOpacity>
            ))
          ) : (
            <Text style={styles.emptyText}>No sessions yet. Start a conversation!</Text>
          )}
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 4, paddingBottom: 32 },
  title: { color: Colors.textPrimary, fontSize: 22, fontWeight: '700' },
  subtitle: { color: Colors.textSecondary, fontSize: 14, marginTop: 2, marginBottom: 12 },
  section: { gap: 8, marginBottom: 20 },
  sectionLabel: {
    color: Colors.textSecondary,
    fontSize: 13,
    fontWeight: '600',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    marginBottom: 4,
  },
  emptyText: { color: Colors.textSecondary, fontSize: 14, fontStyle: 'italic' },
  chip: {
    marginRight: 8,
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderColor: Colors.warning,
    borderWidth: 1,
  },
  chipText: { color: Colors.warning, fontSize: 13, fontWeight: '600' },
  mistakeCard: { gap: 6 },
  mistakeHeader: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  mistakeType: { color: Colors.textPrimary, fontSize: 14, fontWeight: '700' },
  countBadge: {
    backgroundColor: `${Colors.error}33`,
    borderRadius: 10,
    paddingHorizontal: 8,
    paddingVertical: 2,
  },
  countText: { color: Colors.error, fontSize: 12, fontWeight: '700' },
  mistakeDesc: { color: Colors.textSecondary, fontSize: 13 },
  mistakeExample: { color: Colors.textSecondary, fontSize: 12, fontStyle: 'italic' },
  vocabChip: {
    marginRight: 8,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  vocabText: { color: Colors.textPrimary, fontSize: 13 },
  suggestionCard: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12 },
  suggestionText: { flex: 1, color: Colors.textSecondary, fontSize: 13 },
  startBtn: {
    backgroundColor: `${Colors.accent}22`,
    borderColor: Colors.accent,
    borderWidth: 1,
    borderRadius: 16,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  startBtnText: { color: Colors.accent, fontSize: 13, fontWeight: '600' },
  recordCard: { gap: 4 },
  recordHeader: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  recordLanguage: { color: Colors.textSecondary, fontSize: 12, fontWeight: '600' },
  recordFP: { color: Colors.accent, fontSize: 13, fontWeight: '700' },
  recordTopic: { color: Colors.textPrimary, fontSize: 14, fontWeight: '600' },
  recordDate: { color: Colors.textSecondary, fontSize: 12 },
});
