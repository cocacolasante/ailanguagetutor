import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  View, Text, StyleSheet, TouchableOpacity, ScrollView, Alert,
} from 'react-native';
import { router, useLocalSearchParams, useFocusEffect } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQueryClient } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { listeningApi } from '@/api/listening';
import { useAudio } from '@/hooks/useAudio';
import { isPracticeSessionExpired } from '@/utils/errors';
import { cancelStreakReminder } from '@/utils/notifications';
import { queryKeys } from '@/constants/api';
import { ListeningExercise, ListeningCompleteResult } from '@/types/api';

type Phase = 'loading' | 'practice' | 'results' | 'error';

export default function ListeningScreen() {
  const { language, level } = useLocalSearchParams<{ language: string; level: string }>();
  const queryClient = useQueryClient();
  const { playTTS, isPlaying } = useAudio();

  const [phase, setPhase] = useState<Phase>('loading');
  const [exercises, setExercises] = useState<ListeningExercise[]>([]);
  const [currentIndex, setCurrentIndex] = useState(0);
  const [answers, setAnswers] = useState<string[]>([]);
  const [selectedOption, setSelectedOption] = useState<string | null>(null);
  const [hasPlayed, setHasPlayed] = useState(false);
  const [completeResult, setCompleteResult] = useState<ListeningCompleteResult | null>(null);

  const sessionIdRef = useRef<string | null>(null);
  const phaseRef = useRef<Phase>('loading');

  const updatePhase = (p: Phase) => {
    setPhase(p);
    phaseRef.current = p;
  };

  useEffect(() => {
    const lang = language ?? 'it';
    const lvl = parseInt(level ?? '1', 10);
    listeningApi.startSession(lang, lvl)
      .then((session) => {
        sessionIdRef.current = session.session_id;
        setExercises(session.exercises);
        updatePhase('practice');
      })
      .catch((err) => {
        if (isPracticeSessionExpired(err)) {
          updatePhase('error');
        } else {
          Alert.alert('Error', 'Failed to start listening session.');
          router.back();
        }
      });
  }, []);

  useFocusEffect(
    useCallback(() => {
      return () => {
        if (sessionIdRef.current && phaseRef.current !== 'results') {
          listeningApi.completeSession(sessionIdRef.current, []).catch(() => {});
        }
      };
    }, [])
  );

  const handlePlayAudio = async () => {
    const ex = exercises[currentIndex];
    if (!ex) return;
    setHasPlayed(true);
    await playTTS(ex.audioText, language ?? 'it');
  };

  const handleNext = async () => {
    if (!selectedOption) return;
    const newAnswers = [...answers, selectedOption];

    if (currentIndex + 1 >= exercises.length) {
      try {
        const result = await listeningApi.completeSession(sessionIdRef.current!, newAnswers);
        sessionIdRef.current = null;
        setAnswers(newAnswers);
        setCompleteResult(result);
        updatePhase('results');
        queryClient.invalidateQueries({ queryKey: queryKeys.userStats });
        cancelStreakReminder().catch(() => {});
      } catch {
        Alert.alert('Error', 'Failed to complete session.');
      }
    } else {
      setAnswers(newAnswers);
      setCurrentIndex((i) => i + 1);
      setSelectedOption(null);
      setHasPlayed(false);
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
          <Text style={styles.resultsTitle}>Listening Complete! 🎧</Text>
          <Card style={styles.scoreCard}>
            <Text style={styles.scoreText}>{completeResult.score}/{completeResult.total}</Text>
            <Text style={styles.scoreLabel}>Questions Correct</Text>
            <Text style={styles.fpText}>+{completeResult.fluency_points} FP</Text>
          </Card>
          {completeResult.correctAnswers?.length > 0 && (
            <View style={styles.section}>
              <Text style={styles.sectionLabel}>Correct Answers</Text>
              {exercises.map((ex, i) => (
                <Card key={i} style={styles.answerCard}>
                  <Text style={styles.answerQuestion}>{ex.question}</Text>
                  <Text style={styles.answerCorrect}>✅ {completeResult.correctAnswers[i]}</Text>
                  {answers[i] !== completeResult.correctAnswers[i] && (
                    <Text style={styles.answerWrong}>Your answer: {answers[i]}</Text>
                  )}
                </Card>
              ))}
            </View>
          )}
          <Button title="Done" onPress={() => router.back()} style={{ marginTop: 16 }} />
        </ScrollView>
      </SafeAreaView>
    );
  }

  const ex = exercises[currentIndex];
  const progress = exercises.length > 0 ? (currentIndex / exercises.length) : 0;

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()}>
          <Text style={styles.backBtn}>← Back</Text>
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Listening</Text>
        <Text style={styles.progress}>{currentIndex + 1}/{exercises.length}</Text>
      </View>

      <View style={styles.progressBar}>
        <View style={[styles.progressFill, { width: `${progress * 100}%` }]} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {ex && (
          <>
            <Card style={styles.audioCard}>
              <TouchableOpacity style={styles.playBtn} onPress={handlePlayAudio} disabled={isPlaying}>
                <Text style={styles.playBtnText}>{isPlaying ? '⏸ Playing...' : hasPlayed ? '🔁 Replay' : '▶ Play Audio'}</Text>
              </TouchableOpacity>
            </Card>

            {hasPlayed && (
              <>
                <Text style={styles.questionText}>{ex.question}</Text>
                <View style={styles.options}>
                  {ex.options.map((opt) => (
                    <TouchableOpacity
                      key={opt}
                      style={[styles.optionCard, selectedOption === opt && styles.optionSelected]}
                      onPress={() => setSelectedOption(opt)}
                    >
                      <Text style={[styles.optionText, selectedOption === opt && styles.optionTextSelected]}>
                        {opt}
                      </Text>
                    </TouchableOpacity>
                  ))}
                </View>

                <Button
                  title={currentIndex + 1 >= exercises.length ? 'Finish' : 'Next →'}
                  onPress={handleNext}
                  disabled={!selectedOption}
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
  audioCard: { alignItems: 'center', padding: 24 },
  playBtn: {
    backgroundColor: Colors.accent,
    paddingHorizontal: 24,
    paddingVertical: 14,
    borderRadius: 30,
  },
  playBtnText: { color: '#fff', fontSize: 16, fontWeight: '700' },
  questionText: { color: Colors.textPrimary, fontSize: 18, fontWeight: '600' },
  options: { gap: 10 },
  optionCard: {
    backgroundColor: Colors.card,
    borderRadius: 12,
    padding: 14,
    borderWidth: 1,
    borderColor: Colors.border,
  },
  optionSelected: { borderColor: Colors.accent, borderWidth: 1.5 },
  optionText: { color: Colors.textSecondary, fontSize: 15 },
  optionTextSelected: { color: Colors.textPrimary },
  resultsTitle: { color: Colors.textPrimary, fontSize: 24, fontWeight: '700', textAlign: 'center' },
  scoreCard: { alignItems: 'center', gap: 8, padding: 24 },
  scoreText: { color: Colors.textPrimary, fontSize: 48, fontWeight: '800' },
  scoreLabel: { color: Colors.textSecondary, fontSize: 14 },
  fpText: { color: Colors.accent, fontSize: 20, fontWeight: '700' },
  section: { gap: 8 },
  sectionLabel: { color: Colors.textSecondary, fontSize: 13, fontWeight: '600', textTransform: 'uppercase' },
  answerCard: { gap: 6 },
  answerQuestion: { color: Colors.textSecondary, fontSize: 13 },
  answerCorrect: { color: Colors.success, fontSize: 14, fontWeight: '600' },
  answerWrong: { color: Colors.error, fontSize: 13 },
  errorIcon: { fontSize: 48, marginBottom: 12 },
  errorTitle: { color: Colors.textPrimary, fontSize: 20, fontWeight: '700' },
  errorDesc: { color: Colors.textSecondary, fontSize: 14, textAlign: 'center', marginTop: 8 },
});
