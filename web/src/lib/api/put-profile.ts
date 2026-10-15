import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Profile } from '@/types/api';

export function usePutProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Omit<Profile, 'updatedAt'>) =>
      api<Profile>('/profile', { method: 'PUT', body: JSON.stringify(input) }),
    onSuccess: (profile) => queryClient.setQueryData(['profile'], profile),
  });
}
