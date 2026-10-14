import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { kindLabels, newItems } from '@/modules/inbox/utils/new-items';
import type { Channel, InboxItem } from '@/types/api';

export function useInboxEvents(onOpen: (channelId: number) => void) {
  const queryClient = useQueryClient();

  useEffect(() => {
    const source = new EventSource('/inbox/events');
    let before: InboxItem[] | null = null;

    source.addEventListener('inbox', (e) => {
      const items: InboxItem[] = JSON.parse(e.data);
      queryClient.setQueryData(['inbox'], items);
      if (before && Notification.permission === 'granted') {
        for (const item of newItems(before, items)) {
          const name = queryClient
            .getQueryData<Channel[]>(['channels'])
            ?.find((c) => c.id === item.channelId)?.name;
          const n = new Notification(
            `${kindLabels[item.kind]}: ${name ?? `channel ${item.channelId}`}`,
            {
              body: item.text,
            },
          );
          n.onclick = () => {
            window.focus();
            onOpen(item.channelId);
          };
        }
      }
      before = items;
    });
    return () => source.close();
  }, [queryClient, onOpen]);
}
