import React from 'react';
import { TouchableOpacity, StyleSheet } from 'react-native';
import { Colors } from '@/constants/colors';

interface TTSButtonProps {
  onPress: () => void;
  isPlaying?: boolean;
}

export function TTSButton({ onPress, isPlaying = false }: TTSButtonProps) {
  return (
    <TouchableOpacity
      style={[styles.btn, isPlaying && styles.active]}
      onPress={onPress}
      activeOpacity={0.7}
    >
      {/* Simple unicode icons — no icon library dependency */}
      {/* Speaker icon */}
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  btn: {
    width: 32,
    height: 32,
    borderRadius: 16,
    backgroundColor: `${Colors.accent}33`,
    alignItems: 'center',
    justifyContent: 'center',
    marginTop: 6,
  },
  active: {
    backgroundColor: `${Colors.accent}66`,
  },
});
