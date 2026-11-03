import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export type PromptAnswer = 'once' | 'always' | 'deny';

export function useAnswerPrompt(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, answer }: { id: number; answer: PromptAnswer }) =>
      api<void>(`/prompts/${id}/answer`, { method: 'POST', body: JSON.stringify({ answer }) }),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['prompts', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['turns'] }),
      ]),
  });
}
