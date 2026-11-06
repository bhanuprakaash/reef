import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { InboxItem } from '@/types/api';

const inboxQuery = { queryKey: ['inbox'], queryFn: () => api<InboxItem[]>('/inbox') };

export function useInbox() {
  return useQuery(inboxQuery);
}

export function useNeedsYou() {
  return useQuery({
    ...inboxQuery,
    select: (items) => items.filter((i) => i.group === 'needs_you'),
  });
}
