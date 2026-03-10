import { apiClient } from './client';
import { ENDPOINTS } from '@/constants/api';
import { BillingStatus } from '@/types/api';

export const getBillingStatus = () =>
  apiClient.get<BillingStatus>(ENDPOINTS.billingStatus).then((r) => r.data);

export const createCheckout = (plan: string) =>
  apiClient.post<{ url: string }>(ENDPOINTS.billingCheckout, { plan }).then((r) => r.data);

export const createPortalSession = () =>
  apiClient.post<{ url: string }>(ENDPOINTS.billingPortal).then((r) => r.data);
