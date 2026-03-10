import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  View, Text, StyleSheet, TextInput, TouchableOpacity,
  KeyboardAvoidingView, Platform, FlatList, Alert,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Colors } from '@/constants/colors';
import { MessageBubble } from '@/components/conversation/MessageBubble';
import { useSessionStore, ChatMessage } from '@/store/sessionStore';
import { useAuthStore } from '@/store/authStore';
import { useConversationStream } from '@/hooks/useConversationStream';
import { useAudio } from '@/hooks/useAudio';
import { endConversation } from '@/api/conversation';
import { formatDuration } from '@/utils/formatting';
import { cancelStreakReminder } from '@/utils/notifications';

export default function LiveConversationScreen() {
  const { sessionId } = useLocalSearchParams<{ sessionId: string }>();
  const { messages, addMessage, updateLastAssistantMessage, startedAt, language, clearSession } = useSessionStore();
  const token = useAuthStore((s) => s.token);
  const { stream } = useConversationStream();
  const { playTTS, stopAudio, isPlaying } = useAudio();

  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const [playingIndex, setPlayingIndex] = useState<number | null>(null);
  const flatListRef = useRef<FlatList>(null);
  const closeStreamRef = useRef<(() => void) | null>(null);
  const greeted = useRef(false);

  // Timer
  useEffect(() => {
    const interval = setInterval(() => {
      if (startedAt) setElapsed(Math.floor((Date.now() - startedAt) / 1000));
    }, 1000);
    return () => clearInterval(interval);
  }, [startedAt]);

  // Greeting on mount
  useEffect(() => {
    if (greeted.current || !sessionId || !token) return;
    greeted.current = true;
    sendMessage('', true);
  }, [sessionId, token]);

  const sendMessage = useCallback((text: string, isGreet = false) => {
    if (!sessionId || !token) return;
    if (!isGreet && !text.trim()) return;

    if (!isGreet) {
      addMessage({ role: 'user', content: text });
      setInput('');
    }

    setIsStreaming(true);
    let accumulated = '';

    addMessage({ role: 'assistant', content: '' });

    const close = stream(
      sessionId,
      isGreet ? '' : text,
      token,
      (chunk) => {
        accumulated += chunk;
        updateLastAssistantMessage(accumulated);
      },
      () => {
        setIsStreaming(false);
      }
    );
    closeStreamRef.current = close;
  }, [sessionId, token, stream]);

  useEffect(() => {
    flatListRef.current?.scrollToEnd({ animated: true });
  }, [messages]);

  // Cleanup
  useEffect(() => {
    return () => {
      closeStreamRef.current?.();
      stopAudio();
    };
  }, []);

  const handleEnd = async () => {
    Alert.alert('End Session', 'End this conversation?', [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'End',
        style: 'destructive',
        onPress: async () => {
          closeStreamRef.current?.();
          stopAudio();
          try {
            const result = await endConversation(sessionId!);
            clearSession();
            cancelStreakReminder().catch(() => {});
            router.replace({
              pathname: '/(app)/conversation/summary',
              params: { recordId: result.record_id },
            });
          } catch {
            Alert.alert('Error', 'Failed to end session.');
          }
        },
      },
    ]);
  };

  const handlePlayTTS = async (content: string, index: number) => {
    if (playingIndex === index) {
      await stopAudio();
      setPlayingIndex(null);
      return;
    }
    setPlayingIndex(index);
    await playTTS(content, language);
    setPlayingIndex(null);
  };

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.header}>
        <Text style={styles.timer}>{formatDuration(elapsed)}</Text>
        <TouchableOpacity style={styles.endBtn} onPress={handleEnd}>
          <Text style={styles.endBtnText}>End</Text>
        </TouchableOpacity>
      </View>

      <FlatList
        ref={flatListRef}
        data={messages}
        keyExtractor={(_, i) => String(i)}
        renderItem={({ item, index }: { item: ChatMessage; index: number }) => (
          <MessageBubble
            role={item.role}
            content={item.content}
            onPlayTTS={item.role === 'assistant' && item.content
              ? () => handlePlayTTS(item.content, index)
              : undefined}
            isPlaying={playingIndex === index}
          />
        )}
        contentContainerStyle={styles.messageList}
        onContentSizeChange={() => flatListRef.current?.scrollToEnd({ animated: true })}
      />

      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        keyboardVerticalOffset={90}
      >
        <View style={styles.inputRow}>
          <TextInput
            style={styles.input}
            value={input}
            onChangeText={setInput}
            placeholder="Type your message..."
            placeholderTextColor={Colors.textSecondary}
            multiline
            editable={!isStreaming}
          />
          <TouchableOpacity
            style={[styles.sendBtn, (!input.trim() || isStreaming) && styles.sendBtnDisabled]}
            onPress={() => sendMessage(input)}
            disabled={!input.trim() || isStreaming}
          >
            <Text style={styles.sendBtnText}>↑</Text>
          </TouchableOpacity>
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: 16,
    borderBottomWidth: 1,
    borderBottomColor: Colors.border,
  },
  timer: { color: Colors.textSecondary, fontSize: 15, fontWeight: '600' },
  endBtn: {
    backgroundColor: `${Colors.error}22`,
    borderColor: `${Colors.error}55`,
    borderWidth: 1,
    borderRadius: 20,
    paddingHorizontal: 16,
    paddingVertical: 6,
  },
  endBtnText: { color: Colors.error, fontWeight: '600' },
  messageList: { paddingVertical: 12, paddingBottom: 8 },
  inputRow: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    padding: 12,
    gap: 10,
    borderTopWidth: 1,
    borderTopColor: Colors.border,
    backgroundColor: Colors.background,
  },
  input: {
    flex: 1,
    backgroundColor: Colors.card,
    borderWidth: 1,
    borderColor: Colors.border,
    borderRadius: 20,
    paddingHorizontal: 16,
    paddingVertical: 10,
    color: Colors.textPrimary,
    fontSize: 15,
    maxHeight: 120,
  },
  sendBtn: {
    width: 44,
    height: 44,
    borderRadius: 22,
    backgroundColor: Colors.accent,
    alignItems: 'center',
    justifyContent: 'center',
  },
  sendBtnDisabled: { opacity: 0.4 },
  sendBtnText: { color: '#fff', fontSize: 20, fontWeight: '700' },
});
