import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  View, Text, StyleSheet, TextInput, TouchableOpacity, ScrollView, Alert,
} from 'react-native';
import { router, useLocalSearchParams, useFocusEffect } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQueryClient } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { vocabApi } from '@/api/vocab';
import { isPracticeSessionExpired } from '@/utils/errors';
import { cancelStreakReminder } from '@/utils/notifications';
import { queryKeys } from '@/constants/api';
import { VocabWord, VocabCheckResult, VocabCompleteResult } from '@/types/api';

type Phase = 'loading' | 'practice' | 'checking' | 'results' | 'error';

export default function VocabScreen() {
  const { language, level } = useLocalSearchParams<{ language: string; level: string }>();
  const queryClient = useQueryClient();

  const [phase, setPhase] = useState<Phase>('loading');
  const [words, setWords] = useState<VocabWord[]>([]);
  const [currentIndex, setCurrentIndex] = useState(0);
  const [answer, setAnswer] = useState('');
  const [checkResult, setCheckResult] = useState<VocabCheckResult | null>(null);
  const [completeResult, setCompleteResult] = useState<VocabCompleteResult | null>(null);

  const sessionIdRef = useRef<string | null>(null);
  const phaseRef = useRef<Phase>('loading');

  const updatePhase = (p: Phase) => {
    setPhase(p);
    phaseRef.current = p;
  };

  useEffect(() => {
    const lang = language ?? 'it';
    const lvl = parseInt(level ?? '1', 10);
    vocabApi.startSession(lang, lvl)
      .then((session) => {
        sessionIdRef.current = session.session_id;
        setWords(session.words);
        updatePhase('practice');
      })
      .catch((err) => {
        if (isPracticeSessionExpired(err)) {
          updatePhase('error');
        } else {
          Alert.alert('Error', 'Failed to start vocab session.');
          router.back();
        }
      });
  }, []);

  useFocusEffect(
    useCallback(() => {
      return () => {
        if (sessionIdRef.current && phaseRef.current !== 'results') {
          vocabApi.completeSession(sessionIdRef.current).catch(() => {});
        }
      };
    }, [])
  );

  const handleCheck = async () => {
    if (!sessionIdRef.current || !words[currentIndex]) return;
    updatePhase('checking');
    try {
      const result = await vocabApi.checkAnswer(
        sessionIdRef.current,
        words[currentIndex].word,
        answer
      );
      setCheckResult(result);
    } catch (err) {
      if (isPracticeSessionExpired(err)) {
        updatePhase('error');
      } else {
        Alert.alert('Error', 'Failed to check answer.');
        updatePhase('practice');
      }
    }
  };

  const handleNext = async () => {
    if (!sessionIdRef.current || !words[currentIndex] || !checkResult) return;
    try {
      await vocabApi.recordResult(sessionIdRef.current, words[currentIndex].word, checkResult.correct);
    } catch {
      // non-fatal
    }

    const nextIndex = currentIndex + 1;
    if (nextIndex >= words.length) {
      // complete session
      try {
        const result = await vocabApi.completeSession(sessionIdRef.current);
        sessionIdRef.current = null;
        setCompleteResult(result);
        updatePhase('results');
        queryClient.invalidateQueries({ queryKey: queryKeys.userStats });
        cancelStreakReminder().catch(() => {});
      } catch {
        Alert.alert('Error', 'Failed to complete session.');
      }
    } else {
      setCurrentIndex(nextIndex);
      setAnswer('');
      setCheckResult(null);
      updatePhase('practice');
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

  if (phase === 'results' && completeResult) {
    return (
      <SafeAreaView style={styles.safe}>
        <ScrollView contentContainerStyle={styles.content}>
          <Text style={styles.resultsTitle}>Session Complete! 🎉</Text>
          <Card style={styles.scoreCard}>
            <Text style={styles.scoreText}>{completeResult.score}/{completeResult.total}</Text>
            <Text style={styles.scoreLabel}>Words Correct</Text>
            <Text style={styles.fpText}>+{completeResult.fluency_points} FP</Text>
          </Card>
          {completeResult.weak_vocab.length > 0 && (
            <View style={styles.section}>
              <Text style={styles.sectionLabel}>Review These Words</Text>
              {completeResult.weak_vocab.map((w) => (
                <Card key={w} style={styles.weakWord}>
                  <Text style={styles.weakWordText}>{w}</Text>
                </Card>
              ))}
            </View>
          )}
          <Button title="Done" onPress={() => router.back()} style={{ marginTop: 16 }} />
        </ScrollView>
      </SafeAreaView>
    );
  }

  const currentWord = words[currentIndex];
  const progress = words.length > 0 ? (currentIndex / words.length) : 0;

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()}>
          <Text style={styles.backBtn}>← Back</Text>
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Vocabulary</Text>
        <Text style={styles.progress}>{currentIndex + 1}/{words.length}</Text>
      </View>

      {/* Progress bar */}
      <View style={styles.progressBar}>
        <View style={[styles.progressFill, { width: `${progress * 100}%` }]} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {currentWord && (
          <>
            <Card style={styles.wordCard}>
              <Text style={styles.wordText}>{currentWord.word}</Text>
              <Text style={styles.exampleText}>{currentWord.example}</Text>
            </Card>

            {checkResult ? (
              <Card style={[styles.resultCard, checkResult.correct ? styles.resultCorrect : styles.resultWrong]}>
                <Text style={styles.resultIcon}>{checkResult.correct ? '✅' : '❌'}</Text>
                <Text style={styles.resultCorrectAnswer}>
                  Correct: {checkResult.correctAnswer}
                </Text>
                <Text style={styles.resultExplanation}>{checkResult.explanation}</Text>
                <Button title="Next →" onPress={handleNext} style={{ marginTop: 12 }} />
              </Card>
            ) : (
              <>
                <TextInput
                  style={styles.input}
                  value={answer}
                  onChangeText={setAnswer}
                  placeholder="Enter translation..."
                  placeholderTextColor={Colors.textSecondary}
                  editable={phase === 'practice'}
                />
                <Button
                  title={phase === 'checking' ? 'Checking...' : 'Check Answer'}
                  onPress={handleCheck}
                  loading={phase === 'checking'}
                  disabled={!answer.trim() || phase === 'checking'}
                />
              </>
            )}
          </>
        )}
      </ScrollView>
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
  progress: { color: Colors.textSecondary, fontSize: 14 },
  progressBar: { height: 4, backgroundColor: Colors.card },
  progressFill: { height: 4, backgroundColor: Colors.accent },
  content: { padding: 16, gap: 16, paddingBottom: 32 },
  wordCard: { alignItems: 'center', gap: 8, padding: 24 },
  wordText: { color: Colors.textPrimary, fontSize: 28, fontWeight: '700' },
  exampleText: { color: Colors.textSecondary, fontSize: 14, textAlign: 'center' },
  input: {
    backgroundColor: Colors.card,
    borderWidth: 1,
    borderColor: Colors.border,
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12,
    color: Colors.textPrimary,
    fontSize: 16,
  },
  resultCard: { gap: 8, padding: 16, borderWidth: 1.5 },
  resultCorrect: { borderColor: Colors.success },
  resultWrong: { borderColor: Colors.error },
  resultIcon: { fontSize: 24 },
  resultCorrectAnswer: { color: Colors.textPrimary, fontSize: 15, fontWeight: '600' },
  resultExplanation: { color: Colors.textSecondary, fontSize: 14 },
  resultsTitle: { color: Colors.textPrimary, fontSize: 24, fontWeight: '700', textAlign: 'center' },
  scoreCard: { alignItems: 'center', gap: 8, padding: 24 },
  scoreText: { color: Colors.textPrimary, fontSize: 48, fontWeight: '800' },
  scoreLabel: { color: Colors.textSecondary, fontSize: 14 },
  fpText: { color: Colors.accent, fontSize: 20, fontWeight: '700' },
  section: { gap: 8 },
  sectionLabel: { color: Colors.textSecondary, fontSize: 13, fontWeight: '600', textTransform: 'uppercase' },
  weakWord: { paddingVertical: 8, paddingHorizontal: 12 },
  weakWordText: { color: Colors.textPrimary, fontSize: 14 },
  errorIcon: { fontSize: 48, marginBottom: 12 },
  errorTitle: { color: Colors.textPrimary, fontSize: 20, fontWeight: '700' },
  errorDesc: { color: Colors.textSecondary, fontSize: 14, textAlign: 'center', marginTop: 8 },
});
