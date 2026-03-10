import React, { useState } from 'react';
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity, Alert,
} from 'react-native';
import { router } from 'expo-router';
import * as Linking from 'expo-linking';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { Colors } from '@/constants/colors';
import { login } from '@/api/auth';
import { useAuthStore } from '@/store/authStore';
import { extractErrorMessage } from '@/utils/errors';

export default function LoginScreen() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [unverified, setUnverified] = useState(false);
  const { setAuth } = useAuthStore();

  const handleLogin = async () => {
    if (!email || !password) return;
    setLoading(true);
    try {
      const data = await login({ email, password });
      if (data.status === 'email_unverified') {
        setUnverified(true);
        return;
      }
      if (data.checkout_url) {
        await Linking.openURL(data.checkout_url);
        return;
      }
      setAuth(data.user, data.token);
      router.replace('/(app)');
    } catch (err) {
      const msg = extractErrorMessage(err);
      if (msg.includes('email') && msg.includes('verif')) {
        setUnverified(true);
      } else {
        Alert.alert('Login Failed', msg);
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <SafeAreaView style={styles.safe}>
      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        <View style={styles.header}>
          <Text style={styles.logo}>🗣️</Text>
          <Text style={styles.title}>Fluentica AI</Text>
          <Text style={styles.subtitle}>Your AI language tutor</Text>
        </View>

        {unverified && (
          <View style={styles.banner}>
            <Text style={styles.bannerText}>
              Please verify your email before logging in. Check your inbox.
            </Text>
          </View>
        )}

        <View style={styles.form}>
          <Input
            label="Email"
            value={email}
            onChangeText={setEmail}
            keyboardType="email-address"
            autoCapitalize="none"
            autoComplete="email"
            placeholder="you@example.com"
          />
          <Input
            label="Password"
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            placeholder="••••••••"
          />
          <TouchableOpacity onPress={() => router.push('/(auth)/forgot-password')}>
            <Text style={styles.forgotText}>Forgot password?</Text>
          </TouchableOpacity>
          <Button title="Sign In" onPress={handleLogin} loading={loading} />
        </View>

        <View style={styles.footer}>
          <Text style={styles.footerText}>Don't have an account? </Text>
          <TouchableOpacity onPress={() => router.push('/(auth)/register')}>
            <Text style={styles.footerLink}>Sign Up</Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  content: { padding: 24, flexGrow: 1, justifyContent: 'center' },
  header: { alignItems: 'center', marginBottom: 40 },
  logo: { fontSize: 64, marginBottom: 12 },
  title: { color: Colors.textPrimary, fontSize: 28, fontWeight: '700' },
  subtitle: { color: Colors.textSecondary, fontSize: 16, marginTop: 4 },
  banner: {
    backgroundColor: `${Colors.warning}22`,
    borderColor: `${Colors.warning}55`,
    borderWidth: 1,
    borderRadius: 12,
    padding: 12,
    marginBottom: 20,
  },
  bannerText: { color: Colors.warning, fontSize: 14 },
  form: { gap: 16 },
  forgotText: { color: Colors.accent, fontSize: 14, textAlign: 'right' },
  footer: { flexDirection: 'row', justifyContent: 'center', marginTop: 32 },
  footerText: { color: Colors.textSecondary, fontSize: 15 },
  footerLink: { color: Colors.accent, fontSize: 15, fontWeight: '600' },
});
