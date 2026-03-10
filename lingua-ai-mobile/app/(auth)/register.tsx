import React, { useState } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity, Alert,
} from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Colors } from '@/constants/colors';
import { register } from '@/api/auth';
import { extractErrorMessage } from '@/utils/errors';

export default function RegisterScreen() {
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [plan, setPlan] = useState<'trial' | 'immediate'>('trial');
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);

  const handleRegister = async () => {
    if (!email || !username || !password) return;
    setLoading(true);
    try {
      await register({ email, username, password, plan });
      setDone(true);
    } catch (err) {
      Alert.alert('Registration Failed', extractErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  if (done) {
    return (
      <SafeAreaView style={styles.safe}>
        <View style={styles.doneContainer}>
          <Text style={styles.doneIcon}>📧</Text>
          <Text style={styles.doneTitle}>Check your email</Text>
          <Text style={styles.doneText}>
            We sent a verification link to {email}. Tap the link to activate your account and complete setup.
          </Text>
          <Button
            title="Back to Login"
            onPress={() => router.replace('/(auth)/login')}
            variant="secondary"
            style={styles.doneBtn}
          />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        <View style={styles.header}>
          <Text style={styles.title}>Create Account</Text>
          <Text style={styles.subtitle}>Start your language journey</Text>
        </View>

        <View style={styles.form}>
          <Input label="Email" value={email} onChangeText={setEmail} keyboardType="email-address" autoCapitalize="none" placeholder="you@example.com" />
          <Input label="Username" value={username} onChangeText={setUsername} autoCapitalize="none" placeholder="your_name" />
          <Input label="Password" value={password} onChangeText={setPassword} secureTextEntry placeholder="••••••••" />

          <Text style={styles.planLabel}>Choose your plan</Text>
          <View style={styles.planRow}>
            <TouchableOpacity style={[styles.planOption, plan === 'trial' && styles.planSelected]} onPress={() => setPlan('trial')}>
              <Card style={styles.planCard}>
                <Text style={styles.planTitle}>7-Day Trial</Text>
                <Text style={styles.planDesc}>Try free for 7 days</Text>
              </Card>
            </TouchableOpacity>
            <TouchableOpacity style={[styles.planOption, plan === 'immediate' && styles.planSelected]} onPress={() => setPlan('immediate')}>
              <Card style={styles.planCard}>
                <Text style={styles.planTitle}>Subscribe Now</Text>
                <Text style={styles.planDesc}>Full access immediately</Text>
              </Card>
            </TouchableOpacity>
          </View>

          <Button title="Create Account" onPress={handleRegister} loading={loading} />
        </View>

        <View style={styles.footer}>
          <Text style={styles.footerText}>Already have an account? </Text>
          <TouchableOpacity onPress={() => router.replace('/(auth)/login')}>
            <Text style={styles.footerLink}>Sign In</Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 24, flexGrow: 1, justifyContent: 'center' },
  header: { alignItems: 'center', marginBottom: 32 },
  title: { color: Colors.textPrimary, fontSize: 28, fontWeight: '700' },
  subtitle: { color: Colors.textSecondary, fontSize: 16, marginTop: 4 },
  form: { gap: 16 },
  planLabel: { color: Colors.textSecondary, fontSize: 14, fontWeight: '500' },
  planRow: { flexDirection: 'row', gap: 12 },
  planOption: { flex: 1, borderRadius: 16, borderWidth: 2, borderColor: 'transparent' },
  planSelected: { borderColor: Colors.accent },
  planCard: { alignItems: 'center', gap: 4 },
  planTitle: { color: Colors.textPrimary, fontSize: 14, fontWeight: '600' },
  planDesc: { color: Colors.textSecondary, fontSize: 12, textAlign: 'center' },
  footer: { flexDirection: 'row', justifyContent: 'center', marginTop: 32 },
  footerText: { color: Colors.textSecondary, fontSize: 15 },
  footerLink: { color: Colors.accent, fontSize: 15, fontWeight: '600' },
  doneContainer: { flex: 1, justifyContent: 'center', alignItems: 'center', padding: 32, gap: 16 },
  doneIcon: { fontSize: 72 },
  doneTitle: { color: Colors.textPrimary, fontSize: 24, fontWeight: '700' },
  doneText: { color: Colors.textSecondary, fontSize: 15, textAlign: 'center', lineHeight: 22 },
  doneBtn: { marginTop: 16, width: '100%' },
});
