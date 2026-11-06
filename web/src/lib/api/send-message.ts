import { api } from '@/lib/api-client';

export function sendMessage(spaceId: number, text: string, parent?: number) {
  return api<void>(`/spaces/${spaceId}/send`, {
    method: 'POST',
    body: JSON.stringify({ text, parent }),
  });
}
