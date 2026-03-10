import React from 'react';
import { View, Text, StyleSheet, TouchableOpacity } from 'react-native';
import { Colors } from '@/constants/colors';

interface MessageBubbleProps {
  role: 'user' | 'assistant';
  content: string;
  onPlayTTS?: () => void;
  isPlaying?: boolean;
}

export function MessageBubble({ role, content, onPlayTTS, isPlaying }: MessageBubbleProps) {
  const isUser = role === 'user';

  return (
    <View style={[styles.row, isUser && styles.rowUser]}>
      <View style={[styles.bubble, isUser ? styles.userBubble : styles.assistantBubble]}>
        <Text style={[styles.text, isUser && styles.userText]}>{content}</Text>
        {!isUser && onPlayTTS && (
          <TouchableOpacity
            style={[styles.ttsBtn, isPlaying && styles.ttsBtnActive]}
            onPress={onPlayTTS}
            activeOpacity={0.7}
          >
            <Text style={styles.ttsBtnText}>{isPlaying ? '⏹' : '🔊'}</Text>
          </TouchableOpacity>
        )}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { marginVertical: 4, paddingHorizontal: 16, alignItems: 'flex-start' },
  rowUser: { alignItems: 'flex-end' },
  bubble: {
    maxWidth: '80%',
    borderRadius: 18,
    padding: 12,
  },
  userBubble: {
    backgroundColor: Colors.accent,
    borderBottomRightRadius: 4,
  },
  assistantBubble: {
    backgroundColor: Colors.card,
    borderWidth: 1,
    borderColor: Colors.border,
    borderBottomLeftRadius: 4,
  },
  text: { color: Colors.textPrimary, fontSize: 15, lineHeight: 22 },
  userText: { color: '#fff' },
  ttsBtn: {
    marginTop: 8,
    alignSelf: 'flex-start',
    padding: 4,
    borderRadius: 8,
    backgroundColor: `${Colors.accent}22`,
  },
  ttsBtnActive: { backgroundColor: `${Colors.accent}55` },
  ttsBtnText: { fontSize: 14 },
});
