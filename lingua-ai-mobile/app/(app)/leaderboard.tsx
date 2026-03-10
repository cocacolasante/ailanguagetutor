import React, { useState } from 'react';
import { View, Text, StyleSheet, FlatList, RefreshControl } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Colors } from '@/constants/colors';
import { Card } from '@/components/ui/Card';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { getLeaderboard } from '@/api/gamification';
import { useAuthStore } from '@/store/authStore';
import { LeaderboardEntry } from '@/types/api';
import { queryKeys } from '@/constants/api';

const RANK_ICONS = ['🥇', '🥈', '🥉'];

export default function LeaderboardScreen() {
  const user = useAuthStore((s) => s.user);
  const queryClient = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  const { data, isLoading, refetch } = useQuery({
    queryKey: queryKeys.leaderboard,
    queryFn: getLeaderboard,
    staleTime: 60_000,
  });

  const handleRefresh = async () => {
    setRefreshing(true);
    await refetch();
    setRefreshing(false);
  };

  if (isLoading) return <LoadingSpinner style={{ backgroundColor: Colors.background }} />;

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.header}>
        <Text style={styles.title}>Leaderboard</Text>
        <Text style={styles.subtitle}>Top language learners</Text>
      </View>
      <FlatList
        data={data ?? []}
        contentContainerStyle={styles.list}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={handleRefresh}
            tintColor={Colors.accent}
          />
        }
        renderItem={({ item }: { item: LeaderboardEntry }) => {
          const isMe = item.username === user?.username;
          return (
            <Card style={[styles.row, isMe ? styles.rowMe : undefined]}>
              <Text style={styles.rank}>
                {item.rank <= 3 ? RANK_ICONS[item.rank - 1] : `#${item.rank}`}
              </Text>
              <View style={{ flex: 1 }}>
                <Text style={[styles.username, isMe && styles.usernameMe]}>
                  {item.username} {isMe ? '(You)' : ''}
                </Text>
                <Text style={styles.streak}>🔥 {item.streak} day streak</Text>
              </View>
              <Text style={styles.fp}>{item.total_fp} FP</Text>
            </Card>
          );
        }}
        keyExtractor={(item) => item.username}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: Colors.background },
  header: { padding: 16, paddingBottom: 8 },
  title: { color: Colors.textPrimary, fontSize: 22, fontWeight: '700' },
  subtitle: { color: Colors.textSecondary, fontSize: 14, marginTop: 2 },
  list: { padding: 16, paddingTop: 8 },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    marginBottom: 8,
  },
  rowMe: { borderColor: Colors.accent, borderWidth: 1.5 },
  rank: { fontSize: 20, width: 36, textAlign: 'center' },
  username: { color: Colors.textPrimary, fontSize: 15, fontWeight: '600' },
  usernameMe: { color: Colors.accent },
  streak: { color: Colors.textSecondary, fontSize: 12, marginTop: 2 },
  fp: { color: Colors.accent, fontSize: 15, fontWeight: '700' },
});
