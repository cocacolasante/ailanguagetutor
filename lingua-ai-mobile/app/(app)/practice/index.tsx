import React, { useState } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
} from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getLanguages } from '@/api/meta';
import { useAuthStore } from '@/store/authStore';
import { Language } from '@/types/api';
import { queryKeys } from '@/constants/api';

const LEVEL_LABELS = ['', 'Beginner', 'Elementary', 'Intermediate', 'Upper-Int.', 'Fluent'];

const MODES = [
  { key: 'vocab', icon: '📖', title: 'Vocabulary', desc: 'Flashcard practice for new words' },
  { key: 'sentences', icon: '✍️', title: 'Sentences', desc: 'Build and correct sentences' },
  { key: 'listening', icon: '🎧', title: 'Listening', desc: 'Audio comprehension exercises' },
  { key: 'writing', icon: '📝', title: 'Writing Coach', desc: 'AI-guided writing practice' },
];

export default function PracticeHubScreen() {
  const user = useAuthStore((s) => s.user);
  const [language, setLanguage] = useState(user?.pref_language ?? 'it');
  const [level, setLevel] = useState(user?.pref_level ?? 1);

  const { data: languages, isLoading } = useQuery({
    queryKey: queryKeys.languages,
    queryFn: getLanguages,
  });

  if (isLoading) return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <Text style={styles.title}>Practice</Text>
        <Text style={styles.subtitle}>Choose a mode to start</Text>

        <Text style={styles.sectionLabel}>Language</Text>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.hScroll}>
          {languages?.map((lang: Language) => (
            <TouchableOpacity key={lang.code} onPress={() => setLanguage(lang.code)}>
              <Card style={[styles.chip, language === lang.code && styles.chipSelected]}>
                <Text style={styles.chipFlag}>{lang.flag}</Text>
                <Text style={[styles.chipText, language === lang.code && styles.chipTextSelected]}>
                  {lang.name}
                </Text>
              </Card>
            </TouchableOpacity>
          ))}
        </ScrollView>

        <Text style={styles.sectionLabel}>Level</Text>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.hScroll}>
          {[1, 2, 3, 4, 5].map((l) => (
            <TouchableOpacity key={l} onPress={() => setLevel(l)}>
              <Card style={[styles.chip, level === l && styles.chipSelected]}>
                <Text style={[styles.chipText, level === l && styles.chipTextSelected]}>
                  {LEVEL_LABELS[l]}
                </Text>
              </Card>
            </TouchableOpacity>
          ))}
        </ScrollView>

        <Text style={styles.sectionLabel}>Choose Mode</Text>
        <View style={styles.modeGrid}>
          {MODES.map((mode) => (
            <TouchableOpacity
              key={mode.key}
              style={styles.modeCard}
              onPress={() =>
                router.push({
                  pathname: `/(app)/practice/${mode.key}` as never,
                  params: { language, level: String(level) },
                })
              }
            >
              <Text style={styles.modeIcon}>{mode.icon}</Text>
              <Text style={styles.modeTitle}>{mode.title}</Text>
              <Text style={styles.modeDesc}>{mode.desc}</Text>
            </TouchableOpacity>
          ))}
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 12, paddingBottom: 32 },
  title: { color: Colors.textPrimary, fontSize: 22, fontWeight: '700' },
  subtitle: { color: Colors.textSecondary, fontSize: 14, marginTop: 2, marginBottom: 4 },
  sectionLabel: {
    color: Colors.textSecondary,
    fontSize: 13,
    fontWeight: '600',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  hScroll: { marginBottom: 4 },
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    marginRight: 10,
    paddingVertical: 10,
    paddingHorizontal: 14,
  },
  chipSelected: { borderColor: Colors.accent, borderWidth: 1.5 },
  chipFlag: { fontSize: 20 },
  chipText: { color: Colors.textSecondary, fontSize: 14, fontWeight: '500' },
  chipTextSelected: { color: Colors.textPrimary },
  modeGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 12, marginTop: 4 },
  modeCard: {
    width: '47%',
    backgroundColor: Colors.card,
    borderRadius: 16,
    padding: 16,
    borderWidth: 1,
    borderColor: Colors.border,
    gap: 6,
  },
  modeIcon: { fontSize: 28 },
  modeTitle: { color: Colors.textPrimary, fontSize: 15, fontWeight: '700' },
  modeDesc: { color: Colors.textSecondary, fontSize: 12 },
});
