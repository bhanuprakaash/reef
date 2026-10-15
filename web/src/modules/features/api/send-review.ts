import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { messagesQuery } from '@/lib/api/get-messages';
import { channelsQuery } from '@/lib/api/get-channels';
import { teamsQuery } from '@/lib/api/get-teams';
import type { Channel, Message, Team } from '@/types/api';

export type ReviewComment = { path: string; line: number; text: string; removed?: boolean };

// reviewText lists each comment as its path:line (marked "(removed)" for a deleted line)
// followed by its text.
export function reviewText(comments: ReviewComment[]): string {
  return comments
    .map((c) => `${c.path}:${c.line}${c.removed ? ' (removed)' : ''}\n${c.text}`)
    .join('\n\n');
}

// pickChannel returns the feature's channel whose team has a developer role, the most
// recently created one when there are several.
export function pickChannel(channels: Channel[], teams: Team[]): Channel | undefined {
  const developerTeams = new Set(
    teams.filter((t) => t.roles.some((r) => r.name === 'developer')).map((t) => t.name),
  );
  return channels.filter((c) => developerTeams.has(c.team)).at(-1);
}

// pickRoot returns the id of the developer's latest top-level message, the thread a reply
// wakes it in.
export function pickRoot(messages: Message[]): number | undefined {
  return messages.filter((m) => m.parentId === null && m.author === 'developer').at(-1)?.id;
}

// useSendReview sends comments as one user message to the developer's latest thread on
// featureId, or a top-level message starting "@developer" when it has none.
export function useSendReview(featureId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (comments: ReviewComment[]) => {
      const [channels, teams] = await Promise.all([
        queryClient.fetchQuery(channelsQuery),
        queryClient.fetchQuery(teamsQuery),
      ]);
      const channel = pickChannel(
        channels.filter((c) => c.featureId === featureId),
        teams,
      );
      if (!channel) throw new Error('no channel with a developer role on this feature');
      const messages = await queryClient.fetchQuery(messagesQuery(channel.id));
      const parent = pickRoot(messages);
      const text = reviewText(comments);
      await api<void>(`/channels/${channel.id}/send`, {
        method: 'POST',
        body: JSON.stringify({ text: parent ? text : `@developer\n${text}`, parent }),
      });
      return channel.id;
    },
    onSuccess: (channelId) => queryClient.invalidateQueries({ queryKey: ['messages', channelId] }),
  });
}
