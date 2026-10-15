import { useQuery } from '@tanstack/react-query';
import { ApiError, api } from '@/lib/api-client';
import type { Profile } from '@/types/api';

export function useProfile() {
  return useQuery({
    queryKey: ['profile'],
    queryFn: () =>
      api<Profile>('/profile').catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }),
    retry: false,
  });
}
