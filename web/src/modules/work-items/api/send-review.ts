import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { spacesQuery } from '@/lib/api/get-spaces';
import { sendMessage } from '@/lib/api/send-message';
import { botsQuery } from '@/lib/api/get-bots';
import type { SpacePreview, Message, Bot } from '@/types/api';

export type ReviewComment = { path: string; line: number; text: string; removed?: boolean };

// reviewText lists each comment as its path:line (marked "(removed)" for a deleted line)
// followed by its text.
export function reviewText(comments: ReviewComment[]): string {
  return comments
    .map((c) => `${c.path}:${c.line}${c.removed ? ' (removed)' : ''}\n${c.text}`)
    .join('\n\n');
}

// pickSpace returns the Work item's Space with a developer Bot among its members, the most
// recently created one when there are several.
export function pickSpace(spaces: SpacePreview[], bots: Bot[]): SpacePreview | undefined {
  const developers = new Set(bots.filter((b) => b.name === 'developer').map((b) => b.id));
  return spaces.filter((c) => c.members.some((id) => developers.has(id))).at(-1);
}

// pickRoot returns the id of the developer's latest top-level message, the thread a reply
// wakes it in.
export function pickRoot(messages: Message[]): number | undefined {
  return messages.filter((m) => m.parentId === null && m.author === 'developer').at(-1)?.id;
}

// useSendReview sends comments as one user message to the developer's latest thread on
// workItemId, or a top-level message starting "@developer" when it has none.
export function useSendReview(workItemId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (comments: ReviewComment[]) => {
      const [spaces, bots] = await Promise.all([
        queryClient.fetchQuery(spacesQuery()),
        queryClient.fetchQuery(botsQuery),
      ]);
      const space = pickSpace(
        spaces.filter((c) => c.workItemId === workItemId),
        bots,
      );
      if (!space) throw new Error('no Space with a developer role on this Work item');
      const messages = await queryClient.fetchQuery({
        queryKey: ['messages', space.id, 'all'],
        queryFn: () => api<Message[]>(`/messages?space=${space.id}`),
      });
      const parent = pickRoot(messages);
      const text = reviewText(comments);
      await sendMessage(space.id, parent ? text : `@developer\n${text}`, parent);
      return space.id;
    },
    onSuccess: (spaceId) => queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
  });
}
