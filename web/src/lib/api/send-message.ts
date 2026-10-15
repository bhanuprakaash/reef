import { api } from '@/lib/api-client';

export function sendMessage(channelId: number, text: string, parent?: number) {
  return api<void>(`/channels/${channelId}/send`, {
    method: 'POST',
    body: JSON.stringify({ text, parent }),
  });
}
