import React, { useState } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity, Alert,
} from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getLanguages, getTopics, getPersonalities } from '@/api/meta';
import { startConversation } from '@/api/conversation';
import { useSessionStore } from '@/store/sessionStore';
import { Language, Topic, Personality } from '@/types/api';

const LEVEL_LABELS = ['', 'Beginner', 'Elementary', 'Intermediate', 'Upper-Int.', 'Fluent'];

export default function ConversationSetupScreen() {
  const [language, setLanguage] = useState('it');
  const [topic, setTopic] = useState('');
  const [personality, setPersonality] = useState('professor');
  const [level, setLevel] = useState(1);
  const [loading, setLoading] = useState(false);
  const setSession = useSessionStore((s) => s.setSession);

  const { data: languages, isLoading: langsLoading } = useQuery({ queryKey: ['languages'], queryFn: getLanguages });
  const { data: topics, isLoading: topicsLoading } = useQuery({ queryKey: ['topics'], queryFn: getTopics });
  const { data: personalities, isLoading: persLoading } = useQuery({ queryKey: ['personalities'], queryFn: getPersonalities });

  const handleStart = async () => {
    if (!topic) { Alert.alert('Select a topic first'); return; }
    setLoading(true);
    try {
      const session = await startConversation({ language, topic, level, personality });
      setSession(session);
      router.push(`/(app)/conversation/${session.session_id}`);
    } catch {
      Alert.alert('Error', 'Failed to start session. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  if (langsLoading || topicsLoading || persLoading) {
    return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;
  }

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <Text style={styles.title}>New Conversation</Text>

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

        <Text style={styles.sectionLabel}>Tutor Personality</Text>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.hScroll}>
          {personalities?.map((p: Personality) => (
            <TouchableOpacity key={p.id} onPress={() => setPersonality(p.id)}>
              <Card style={[styles.chip, personality === p.id && styles.chipSelected]}>
                <Text style={styles.chipFlag}>{p.icon}</Text>
                <Text style={[styles.chipText, personality === p.id && styles.chipTextSelected]}>
                  {p.name}
                </Text>
              </Card>
            </TouchableOpacity>
          ))}
        </ScrollView>

        <Text style={styles.sectionLabel}>Topic</Text>
        <View style={styles.topicGrid}>
          {topics?.map((t: Topic) => (
            <TouchableOpacity
              key={t.id}
              style={[styles.topicCard, topic === t.id && styles.topicSelected]}
              onPress={() => setTopic(t.id)}
            >
              <Text style={styles.topicIcon}>{t.icon}</Text>
              <Text style={[styles.topicName, topic === t.id && styles.topicNameSelected]}>
                {t.name}
              </Text>
            </TouchableOpacity>
          ))}
        </View>

        <Button
          title="Start Conversation"
          onPress={handleStart}
          loading={loading}
          disabled={!topic}
          style={styles.startBtn}
        />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 12, paddingBottom: 32 },
  title: { color: Colors.textPrimary, fontSize: 22, fontWeight: '700', marginBottom: 4 },
  sectionLabel: { color: Colors.textSecondary, fontSize: 13, fontWeight: '600', textTransform: 'uppercase', letterSpacing: 0.5 },
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
  topicGrid: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  topicCard: {
    width: '47%',
    backgroundColor: Colors.card,
    borderRadius: 14,
    padding: 14,
    alignItems: 'center',
    gap: 6,
    borderWidth: 1,
    borderColor: Colors.border,
  },
  topicSelected: { borderColor: Colors.accent, borderWidth: 1.5 },
  topicIcon: { fontSize: 26 },
  topicName: { color: Colors.textSecondary, fontSize: 13, textAlign: 'center', fontWeight: '500' },
  topicNameSelected: { color: Colors.textPrimary },
  startBtn: { marginTop: 8 },
});
