import React from 'react';
import {
  View, Text, StyleSheet, ScrollView,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { AchievementChip } from '@/components/dashboard/AchievementChip';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getConversationRecord } from '@/api/conversation';

export default function SummaryScreen() {
  const { recordId } = useLocalSearchParams<{ recordId: string }>();

  const { data: record, isLoading } = useQuery({
    queryKey: ['record', recordId],
    queryFn: () => getConversationRecord(recordId!),
    enabled: !!recordId,
  });

  if (isLoading) return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;
  if (!record) return null;

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <View style={styles.fpBox}>
          <Text style={styles.fpLabel}>Fluency Points Earned</Text>
          <Text style={styles.fpValue}>+{record.fp_earned}</Text>
        </View>

        <Card style={styles.metaCard}>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Language</Text>
            <Badge label={record.language.toUpperCase()} />
          </View>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Topic</Text>
            <Text style={styles.metaValue}>{record.topic_name}</Text>
          </View>
          <View style={styles.metaRow}>
            <Text style={styles.metaLabel}>Messages</Text>
            <Text style={styles.metaValue}>{record.message_count}</Text>
          </View>
        </Card>

        {record.summary && (
          <Card style={styles.section}>
            <Text style={styles.sectionTitle}>AI Summary</Text>
            <Text style={styles.sectionText}>{record.summary}</Text>
          </Card>
        )}

        {record.vocabulary_learned?.length > 0 && (
          <Card style={styles.section}>
            <Text style={styles.sectionTitle}>Vocabulary Learned</Text>
            <View style={styles.chips}>
              {record.vocabulary_learned.map((word, i) => (
                <Badge key={i} label={word} color={Colors.success} />
              ))}
            </View>
          </Card>
        )}

        {record.grammar_corrections?.length > 0 && (
          <Card style={styles.section}>
            <Text style={styles.sectionTitle}>Grammar Corrections</Text>
            {record.grammar_corrections.map((c, i) => (
              <Text key={i} style={styles.listItem}>• {c}</Text>
            ))}
          </Card>
        )}

        {record.suggested_next_lessons?.length > 0 && (
          <Card style={styles.section}>
            <Text style={styles.sectionTitle}>Suggested Next Steps</Text>
            {record.suggested_next_lessons.map((s, i) => (
              <Text key={i} style={styles.listItem}>• {s}</Text>
            ))}
          </Card>
        )}

        <Button
          title="Back to Dashboard"
          onPress={() => router.replace('/(app)')}
          style={styles.backBtn}
        />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 16, paddingBottom: 32 },
  fpBox: { alignItems: 'center', paddingVertical: 24 },
  fpLabel: { color: Colors.textSecondary, fontSize: 14, fontWeight: '600' },
  fpValue: { color: Colors.accent, fontSize: 56, fontWeight: '800', marginTop: 4 },
  metaCard: { gap: 10 },
  metaRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  metaLabel: { color: Colors.textSecondary, fontSize: 14 },
  metaValue: { color: Colors.textPrimary, fontSize: 14, fontWeight: '600' },
  section: { gap: 10 },
  sectionTitle: { color: Colors.textPrimary, fontSize: 15, fontWeight: '700' },
  sectionText: { color: Colors.textSecondary, fontSize: 14, lineHeight: 22 },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  listItem: { color: Colors.textSecondary, fontSize: 14, lineHeight: 22 },
  backBtn: { marginTop: 8 },
});
