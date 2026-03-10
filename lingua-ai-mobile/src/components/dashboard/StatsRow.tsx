import React from 'react';
import { View, Text, StyleSheet } from 'react-native';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';

interface StatsRowProps {
  streak: number;
  totalFP: number;
  languageCount: number;
}

export function StatsRow({ streak, totalFP, languageCount }: StatsRowProps) {
  return (
    <View style={styles.row}>
      <Card style={styles.stat}>
        <Text style={styles.statIcon}>🔥</Text>
        <Text style={styles.statValue}>{streak}</Text>
        <Text style={styles.statLabel}>Streak</Text>
      </Card>
      <Card style={styles.stat}>
        <Text style={styles.statIcon}>⭐</Text>
        <Text style={styles.statValue}>{totalFP}</Text>
        <Text style={styles.statLabel}>Total FP</Text>
      </Card>
      <Card style={styles.stat}>
        <Text style={styles.statIcon}>🌍</Text>
        <Text style={styles.statValue}>{languageCount}</Text>
        <Text style={styles.statLabel}>Languages</Text>
      </Card>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', gap: 10 },
  stat: {
    flex: 1,
    alignItems: 'center',
    paddingVertical: 16,
    gap: 4,
  },
  statIcon: { fontSize: 22 },
  statValue: { color: Colors.textPrimary, fontSize: 22, fontWeight: '700' },
  statLabel: { color: Colors.textSecondary, fontSize: 12 },
});
