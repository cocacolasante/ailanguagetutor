import React from 'react';
import { View, StyleSheet, ViewStyle } from 'react-native';
import { Colors } from '@/constants/colors';

interface ProgressBarProps {
  value: number; // 0-1
  color?: string;
  style?: ViewStyle;
  height?: number;
}

export function ProgressBar({ value, color = Colors.accent, style, height = 6 }: ProgressBarProps) {
  const pct = Math.min(1, Math.max(0, value));
  return (
    <View style={[styles.track, { height }, style]}>
      <View style={[styles.fill, { width: `${pct * 100}%`, backgroundColor: color, height }]} />
    </View>
  );
}

const styles = StyleSheet.create({
  track: {
    backgroundColor: Colors.border,
    borderRadius: 100,
    overflow: 'hidden',
    width: '100%',
  },
  fill: { borderRadius: 100 },
});
