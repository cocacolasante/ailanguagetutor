import React from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getConversationRecord } from '@/api/gamification';
import { queryKeys } from '@/constants/api';

export default function RecordDetailScreen() {
  const { recordId } = useLocalSearchParams<{ recordId: string }>();

  const { data: record, isLoading, error } = useQuery({
    queryKey: queryKeys.record(recordId ?? ''),
    queryFn: () => getConversationRecord(recordId!),
    enabled: !!recordId,
  });

  if (isLoading) return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;

  if (error || !record) {
    return (
      <SafeAreaView style={styles.safe}>
        <View style={styles.centered}>
          <Text style={styles.errorText}>Failed to load session record.</Text>
          <TouchableOpacity onPress={() => router.back()}>
            <Text style={styles.backLink}>← Go Back</Text>
          </TouchableOpacity>
        </View>
      </SafeAreaView>
    );
  }

  const durationMins = Math.floor(record.duration_secs / 60);
  const durationSecs = record.duration_secs % 60;

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()}>
          <Text style={styles.backBtn}>← Back</Text>
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Session Detail</Text>
        <View style={{ width: 60 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        {/* Meta */}
        <Card style={styles.metaCard}>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Language</Text>
            <Text style={styles.metaValue}>{record.language.toUpperCase()}</Text>
          </View>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Level</Text>
            <Text style={styles.metaValue}>{record.level}</Text>
          </View>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Topic</Text>
            <Text style={styles.metaValue}>{record.topic_name}</Text>
          </View>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Duration</Text>
            <Text style={styles.metaValue}>{durationMins}m {durationSecs}s</Text>
          </View>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Messages</Text>
            <Text style={styles.metaValue}>{record.message_count}</Text>
          </View>
          <View style={[styles.metaRow, { borderBottomWidth: 0 }]}>
            <Text style={styles.metaLabel}>FP Earned</Text>
            <Text style={[styles.metaValue, styles.fpValue]}>+{record.fp_earned}</Text>
          </View>
        </Card>

        {/* AI Summary */}
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>AI Summary</Text>
          <Card>
            <Text style={styles.summaryText}>{record.summary}</Text>
          </Card>
        </View>

        {/* Vocabulary */}
        {record.vocabulary_learned.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionLabel}>Vocabulary Learned</Text>
            <View style={styles.tagRow}>
              {record.vocabulary_learned.map((word) => (
                <View key={word} style={styles.tag}>
                  <Text style={styles.tagText}>{word}</Text>
                </View>
              ))}
            </View>
          </View>
        )}

        {/* Grammar Corrections */}
        {record.grammar_corrections.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionLabel}>Grammar Corrections</Text>
            {record.grammar_corrections.map((c, i) => (
              <Card key={i} style={styles.correctionCard}>
                <Text style={styles.correctionBullet}>•</Text>
                <Text style={styles.correctionText}>{c}</Text>
              </Card>
            ))}
          </View>
        )}

        {/* Misspellings */}
        {record.misspellings.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionLabel}>Misspellings</Text>
            <View style={styles.tagRow}>
              {record.misspellings.map((word) => (
                <View key={word} style={[styles.tag, styles.tagError]}>
                  <Text style={[styles.tagText, styles.tagTextError]}>{word}</Text>
                </View>
              ))}
            </View>
          </View>
        )}

        {/* Topics Discussed */}
        {record.topics_discussed.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionLabel}>Topics Discussed</Text>
            <View style={styles.tagRow}>
              {record.topics_discussed.map((t) => (
                <View key={t} style={[styles.tag, styles.tagAccent]}>
                  <Text style={[styles.tagText, styles.tagTextAccent]}>{t}</Text>
                </View>
              ))}
            </View>
          </View>
        )}

        {/* Suggested Next Lessons */}
        {record.suggested_next_lessons.length > 0 && (
          <View style={styles.section}>
            <Text style={styles.sectionLabel}>Suggested Next Lessons</Text>
            {record.suggested_next_lessons.map((s, i) => (
              <Card key={i} style={styles.suggestionCard}>
                <Text style={styles.suggestionText}>{s}</Text>
              </Card>
            ))}
          </View>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  centered: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 16 },
  errorText: { color: Colors.textSecondary, fontSize: 15 },
  backLink: { color: Colors.accent, fontSize: 15 },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: 16,
    borderBottomWidth: 1,
    borderBottomColor: Colors.border,
  },
  backBtn: { color: Colors.accent, fontSize: 15, width: 60 },
  headerTitle: { color: Colors.textPrimary, fontSize: 16, fontWeight: '700' },
  content: { padding: 16, gap: 4, paddingBottom: 40 },
  metaCard: { gap: 0 },
  metaRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingVertical: 8,
    borderBottomWidth: 1,
    borderBottomColor: Colors.border,
  },
  metaLabel: { color: Colors.textSecondary, fontSize: 14 },
  metaValue: { color: Colors.textPrimary, fontSize: 14, fontWeight: '600' },
  fpValue: { color: Colors.accent },
  section: { gap: 8, marginTop: 16 },
  sectionLabel: {
    color: Colors.textSecondary,
    fontSize: 13,
    fontWeight: '600',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  summaryText: { color: Colors.textPrimary, fontSize: 14, lineHeight: 22 },
  tagRow: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  tag: {
    backgroundColor: `${Colors.success}22`,
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 4,
  },
  tagText: { color: Colors.success, fontSize: 13 },
  tagError: { backgroundColor: `${Colors.error}22` },
  tagTextError: { color: Colors.error },
  tagAccent: { backgroundColor: `${Colors.accent}22` },
  tagTextAccent: { color: Colors.accent },
  correctionCard: { flexDirection: 'row', gap: 8, alignItems: 'flex-start' },
  correctionBullet: { color: Colors.error, fontSize: 16, lineHeight: 20 },
  correctionText: { flex: 1, color: Colors.textPrimary, fontSize: 13, lineHeight: 20 },
  suggestionCard: {},
  suggestionText: { color: Colors.textSecondary, fontSize: 13 },
});
