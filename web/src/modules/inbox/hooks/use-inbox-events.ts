import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import { useLocation } from 'react-router';
import { kindLabels, newItems, noticeKinds } from '@/modules/inbox/utils/new-items';
import type { Space, InboxItem } from '@/types/api';

export function useInboxEvents(onOpen: (item: InboxItem) => void) {
  const queryClient = useQueryClient();
  const { pathname } = useLocation();
  const open = useRef(pathname);

  useEffect(() => {
    open.current = pathname;
  }, [pathname]);

  useEffect(() => {
    const source = new EventSource('/inbox/events');
    let before: InboxItem[] | null = null;

    source.addEventListener('inbox', (e) => {
      const items: InboxItem[] = JSON.parse(e.data);
      queryClient.setQueryData(['inbox'], items);
      if (before && Notification.permission === 'granted') {
        for (const item of newItems(before, items)) {
          if (noticeKinds.includes(item.kind)) continue;
          if (
            document.visibilityState === 'visible' &&
            open.current === `/spaces/${item.spaceId}`
          ) {
            continue;
          }
          const name = queryClient
            .getQueryData<Space[]>(['spaces'])
            ?.find((c) => c.id === item.spaceId)?.name;
          const n = new Notification(
            `${kindLabels[item.kind]}: ${name ?? (item.spaceId === 0 ? `Work item ${item.workItemId}` : `Space ${item.spaceId}`)}`,
            {
              body: item.text,
            },
          );
          n.onclick = () => {
            window.focus();
            onOpen(item);
          };
        }
      }
      before = items;
    });
    return () => source.close();
  }, [queryClient, onOpen]);
}
