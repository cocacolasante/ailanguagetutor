import React, { useState } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity, Alert,
} from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { useAuthStore } from '@/store/authStore';
import { logout, updatePreferences } from '@/api/auth';

const LANGUAGES = [
  { code: 'it', name: 'Italian', flag: '🇮🇹' },
  { code: 'es', name: 'Spanish', flag: '🇪🇸' },
  { code: 'pt', name: 'Portuguese', flag: '🇧🇷' },
  { code: 'en', name: 'English', flag: '🇺🇸' },
];

const PERSONALITIES = [
  { id: 'professor', name: 'Professor', icon: '👨‍🏫' },
  { id: 'friendly-partner', name: 'Friendly Partner', icon: '👫' },
  { id: 'bartender', name: 'Bartender', icon: '🍺' },
  { id: 'business-executive', name: 'Business Executive', icon: '💼' },
  { id: 'travel-guide', name: 'Travel Guide', icon: '🗺️' },
];

const LEVEL_LABELS = ['', 'Beginner', 'Elementary', 'Intermediate', 'Upper-Int.', 'Fluent'];

export default function SettingsScreen() {
  const { user, clearAuth, setUser } = useAuthStore();
  const [langPref, setLangPref] = useState(user?.pref_language ?? 'it');
  const [levelPref, setLevelPref] = useState(user?.pref_level ?? 1);
  const [persPref, setPersPref] = useState(user?.pref_personality ?? 'professor');
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await updatePreferences({ pref_language: langPref, pref_level: levelPref, pref_personality: persPref });
      if (user) setUser({ ...user, pref_language: langPref, pref_level: levelPref, pref_personality: persPref });
      Alert.alert('Saved', 'Preferences updated.');
    } catch {
      Alert.alert('Error', 'Failed to save preferences.');
    } finally {
      setSaving(false);
    }
  };

  const handleLogout = () => {
    Alert.alert('Sign Out', 'Are you sure?', [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Sign Out',
        style: 'destructive',
        onPress: async () => {
          try { await logout(); } catch {}
          clearAuth();
          router.replace('/(auth)/login');
        },
      },
    ]);
  };

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <Text style={styles.title}>Settings</Text>

        <Text style={styles.label}>Default Language</Text>
        <View style={styles.optionRow}>
          {LANGUAGES.map((l) => (
            <TouchableOpacity
              key={l.code}
              style={[styles.option, langPref === l.code && styles.optionSelected]}
              onPress={() => setLangPref(l.code)}
            >
              <Card style={styles.optionCard}>
                <Text style={styles.optionIcon}>{l.flag}</Text>
                <Text style={[styles.optionText, langPref === l.code && styles.optionTextSelected]}>{l.name}</Text>
              </Card>
            </TouchableOpacity>
          ))}
        </View>

        <Text style={styles.label}>Default Level</Text>
        <View style={styles.optionRow}>
          {[1, 2, 3, 4, 5].map((l) => (
            <TouchableOpacity
              key={l}
              style={[styles.levelOption, levelPref === l && styles.optionSelected]}
              onPress={() => setLevelPref(l)}
            >
              <Text style={[styles.levelText, levelPref === l && styles.optionTextSelected]}>
                {LEVEL_LABELS[l]}
              </Text>
            </TouchableOpacity>
          ))}
        </View>

        <Text style={styles.label}>Default Personality</Text>
        <View style={styles.persColumn}>
          {PERSONALITIES.map((p) => (
            <TouchableOpacity
              key={p.id}
              style={[styles.persOption, persPref === p.id && styles.persSelected]}
              onPress={() => setPersPref(p.id)}
            >
              <Text style={styles.persIcon}>{p.icon}</Text>
              <Text style={[styles.persText, persPref === p.id && styles.optionTextSelected]}>{p.name}</Text>
            </TouchableOpacity>
          ))}
        </View>

        <Button title="Save Preferences" onPress={handleSave} loading={saving} style={{ marginTop: 8 }} />

        <View style={styles.divider} />

        <Button title="Sign Out" onPress={handleLogout} variant="secondary"
          textStyle={{ color: Colors.error }} />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 16, gap: 12, paddingBottom: 32 },
  title: { color: Colors.textPrimary, fontSize: 22, fontWeight: '700', marginBottom: 4 },
  label: { color: Colors.textSecondary, fontSize: 13, fontWeight: '600', textTransform: 'uppercase', letterSpacing: 0.5 },
  optionRow: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  option: { borderRadius: 14, borderWidth: 2, borderColor: 'transparent' },
  optionSelected: { borderColor: Colors.accent },
  optionCard: { alignItems: 'center', paddingVertical: 10, paddingHorizontal: 12, gap: 4 },
  optionIcon: { fontSize: 22 },
  optionText: { color: Colors.textSecondary, fontSize: 13 },
  optionTextSelected: { color: Colors.textPrimary, fontWeight: '600' },
  levelOption: {
    backgroundColor: Colors.card,
    borderRadius: 10,
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderWidth: 2,
    borderColor: 'transparent',
  },
  levelText: { color: Colors.textSecondary, fontSize: 13, fontWeight: '500' },
  persColumn: { gap: 8 },
  persOption: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    backgroundColor: Colors.card,
    borderRadius: 14,
    padding: 14,
    borderWidth: 2,
    borderColor: 'transparent',
  },
  persSelected: { borderColor: Colors.accent },
  persIcon: { fontSize: 22 },
  persText: { color: Colors.textSecondary, fontSize: 14, fontWeight: '500' },
  divider: { height: 1, backgroundColor: Colors.border, marginVertical: 8 },
});
