import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { InboxItem } from '@/types/api';

export function useInbox() {
  return useQuery({ queryKey: ['inbox'], queryFn: () => api<InboxItem[]>('/inbox') });
}
