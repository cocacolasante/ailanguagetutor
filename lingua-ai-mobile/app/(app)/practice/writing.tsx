import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  View, Text, StyleSheet, TextInput, TouchableOpacity, FlatList,
  KeyboardAvoidingView, Platform, Alert, Modal, ScrollView,
} from 'react-native';
import { router, useLocalSearchParams, useFocusEffect } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQueryClient } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { MessageBubble } from '@/components/conversation/MessageBubble';
import { writingApi } from '@/api/writing';
import { useWritingStream } from '@/hooks/useWritingStream';
import { cancelStreakReminder } from '@/utils/notifications';
import { useAuthStore } from '@/store/authStore';
import { isPracticeSessionExpired } from '@/utils/errors';
import { queryKeys } from '@/constants/api';
import { WritingCompleteResult } from '@/types/api';

type Phase = 'loading' | 'practice' | 'error';

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

export default function WritingScreen() {
  const { language, level } = useLocalSearchParams<{ language: string; level: string }>();
  const queryClient = useQueryClient();
  const token = useAuthStore((s) => s.token);
  const { stream } = useWritingStream();

  const [phase, setPhase] = useState<Phase>('loading');
  const [prompt, setPrompt] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [showModal, setShowModal] = useState(false);
  const [completeResult, setCompleteResult] = useState<WritingCompleteResult | null>(null);

  const sessionIdRef = useRef<string | null>(null);
  const phaseRef = useRef<Phase>('loading');
  const closeStreamRef = useRef<(() => void) | null>(null);
  const flatListRef = useRef<FlatList>(null);

  const updatePhase = (p: Phase) => {
    setPhase(p);
    phaseRef.current = p;
  };

  useEffect(() => {
    const lang = language ?? 'it';
    const lvl = parseInt(level ?? '1', 10);
    writingApi.startSession(lang, lvl)
      .then((session) => {
        sessionIdRef.current = session.session_id;
        setPrompt(session.prompt);
        updatePhase('practice');
        setMessages([{
          role: 'assistant',
          content: `Writing prompt: ${session.prompt}`,
        }]);
      })
      .catch((err) => {
        if (isPracticeSessionExpired(err)) {
          updatePhase('error');
        } else {
          Alert.alert('Error', 'Failed to start writing session.');
          router.back();
        }
      });

    return () => {
      closeStreamRef.current?.();
    };
  }, []);

  useFocusEffect(
    useCallback(() => {
      return () => {
        if (sessionIdRef.current && phaseRef.current !== 'error') {
          writingApi.completeSession(sessionIdRef.current).catch(() => {});
          sessionIdRef.current = null;
        }
      };
    }, [])
  );

  const sendMessage = () => {
    if (!sessionIdRef.current || !token || !input.trim() || isStreaming) return;
    const text = input.trim();
    setInput('');
    setMessages((prev) => [...prev, { role: 'user', content: text }]);
    setIsStreaming(true);

    let accumulated = '';
    setMessages((prev) => [...prev, { role: 'assistant', content: '' }]);

    const close = stream(
      sessionIdRef.current,
      text,
      token,
      (chunk) => {
        accumulated += chunk;
        setMessages((prev) => {
          const next = [...prev];
          next[next.length - 1] = { role: 'assistant', content: accumulated };
          return next;
        });
      },
      () => {
        setIsStreaming(false);
      }
    );
    closeStreamRef.current = close;
  };

  const handleEndSession = async () => {
    if (!sessionIdRef.current) return;
    closeStreamRef.current?.();
    try {
      const result = await writingApi.completeSession(sessionIdRef.current);
      sessionIdRef.current = null;
      setCompleteResult(result);
      setShowModal(true);
      queryClient.invalidateQueries({ queryKey: queryKeys.userStats });
      cancelStreakReminder().catch(() => {});
    } catch {
      Alert.alert('Error', 'Failed to complete session.');
    }
  };

  if (phase === 'loading') {
    return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;
  }

  if (phase === 'error') {
    return (
      <SafeAreaView style={styles.safe}>
        <View style={styles.centered}>
          <Text style={styles.errorIcon}>⏰</Text>
          <Text style={styles.errorTitle}>Session Expired</Text>
          <Text style={styles.errorDesc}>Your practice session has expired. Start a new one.</Text>
          <Button title="Start Again" onPress={() => router.back()} style={{ marginTop: 16 }} />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()}>
          <Text style={styles.backBtn}>← Back</Text>
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Writing Coach</Text>
        <TouchableOpacity style={styles.endBtn} onPress={handleEndSession}>
          <Text style={styles.endBtnText}>End</Text>
        </TouchableOpacity>
      </View>

      <FlatList
        ref={flatListRef}
        data={messages}
        keyExtractor={(_, i) => String(i)}
        renderItem={({ item }) => (
          <MessageBubble role={item.role} content={item.content} />
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
            placeholder="Write your response..."
            placeholderTextColor={Colors.textSecondary}
            multiline
            editable={!isStreaming}
          />
          <TouchableOpacity
            style={[styles.sendBtn, (!input.trim() || isStreaming) && styles.sendBtnDisabled]}
            onPress={sendMessage}
            disabled={!input.trim() || isStreaming}
          >
            <Text style={styles.sendBtnText}>↑</Text>
          </TouchableOpacity>
        </View>
      </KeyboardAvoidingView>

      <Modal visible={showModal} transparent animationType="slide">
        <View style={styles.modalOverlay}>
          <View style={styles.modalCard}>
            <Text style={styles.modalTitle}>Writing Complete! 📝</Text>
            {completeResult && (
              <ScrollView style={{ maxHeight: 400 }}>
                <Text style={styles.fpText}>+{completeResult.fluency_points} FP earned</Text>
                <Text style={styles.feedbackLabel}>Feedback</Text>
                <Text style={styles.feedbackText}>{completeResult.feedback}</Text>
                {completeResult.corrections.length > 0 && (
                  <>
                    <Text style={styles.feedbackLabel}>Corrections</Text>
                    {completeResult.corrections.map((c, i) => (
                      <Card key={i} style={styles.correctionCard}>
                        <Text style={styles.correctionText}>{c}</Text>
                      </Card>
                    ))}
                  </>
                )}
              </ScrollView>
            )}
            <Button title="Done" onPress={() => { setShowModal(false); router.back(); }} style={{ marginTop: 16 }} />
          </View>
        </View>
      </Modal>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  centered: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: 24 },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: 16,
    borderBottomWidth: 1,
    borderBottomColor: Colors.border,
  },
  backBtn: { color: Colors.accent, fontSize: 15 },
  headerTitle: { color: Colors.textPrimary, fontSize: 16, fontWeight: '700' },
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
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.7)',
    justifyContent: 'flex-end',
  },
  modalCard: {
    backgroundColor: Colors.card,
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    padding: 24,
    paddingBottom: 40,
  },
  modalTitle: { color: Colors.textPrimary, fontSize: 20, fontWeight: '700', marginBottom: 12 },
  fpText: { color: Colors.accent, fontSize: 18, fontWeight: '700', marginBottom: 12 },
  feedbackLabel: { color: Colors.textSecondary, fontSize: 12, fontWeight: '600', textTransform: 'uppercase', marginBottom: 6, marginTop: 12 },
  feedbackText: { color: Colors.textPrimary, fontSize: 14 },
  correctionCard: { marginBottom: 6 },
  correctionText: { color: Colors.textPrimary, fontSize: 13 },
  errorIcon: { fontSize: 48, marginBottom: 12 },
  errorTitle: { color: Colors.textPrimary, fontSize: 20, fontWeight: '700' },
  errorDesc: { color: Colors.textSecondary, fontSize: 14, textAlign: 'center', marginTop: 8 },
});
