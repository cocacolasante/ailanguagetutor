import React from 'react';
import { View, Text, StyleSheet } from 'react-native';
import { Colors } from '@/constants/colors';

interface AchievementChipProps {
  id: string;
  name?: string;
  icon?: string;
  earned?: boolean;
}

export function AchievementChip({ id, name, icon, earned = true }: AchievementChipProps) {
  return (
    <View style={[styles.chip, !earned && styles.locked]}>
      <Text style={styles.icon}>{icon ?? '🏅'}</Text>
      {name && <Text style={[styles.name, !earned && styles.lockedText]}>{name}</Text>}
    </View>
  );
}

const styles = StyleSheet.create({
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: `${Colors.accent}22`,
    borderColor: `${Colors.accent}55`,
    borderWidth: 1,
    borderRadius: 20,
    paddingHorizontal: 10,
    paddingVertical: 5,
  },
  locked: {
    backgroundColor: `${Colors.textSecondary}11`,
    borderColor: `${Colors.textSecondary}33`,
    opacity: 0.5,
  },
  icon: { fontSize: 14 },
  name: { color: Colors.accentLight, fontSize: 12, fontWeight: '600' },
  lockedText: { color: Colors.textSecondary },
});
