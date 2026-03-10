import { useQuery } from '@tanstack/react-query';
import { getBillingStatus } from '@/api/billing';
import { useAuthStore } from '@/store/authStore';

export function useSubscription() {
  const token = useAuthStore((s) => s.token);

  const query = useQuery({
    queryKey: ['billing-status'],
    queryFn: getBillingStatus,
    enabled: !!token,
    staleTime: 60_000,
  });

  const status = query.data?.subscription_status ?? '';
  const hasFullAccess = query.data?.has_full_access ?? false;
  const hasConversationAccess = query.data?.has_conversation_access ?? false;
  const isTrialing = status === 'trialing';
  const isActive = status === 'active' || status === 'free';

  return {
    ...query,
    status,
    hasFullAccess,
    hasConversationAccess,
    isTrialing,
    isActive,
    trialEndsAt: query.data?.trial_ends_at ?? null,
  };
}
