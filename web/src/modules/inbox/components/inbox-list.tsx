import { useState } from 'react';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';
import { useChannels } from '@/lib/api/get-channels';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { InboxRow } from '@/modules/inbox/components/inbox-row';
import { itemKey } from '@/modules/inbox/utils/new-items';
import type { InboxItem } from '@/types/api';

type Filter = 'all' | InboxItem['kind'];

const filters: { value: Filter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'approval', label: 'Approvals' },
  { value: 'waiting', label: 'Waiting' },
  { value: 'paused', label: 'Paused' },
  { value: 'failed', label: 'Failed' },
];

export function InboxList() {
  const inbox = useInbox();
  const channels = useChannels();
  const [permission, setPermission] = useState(Notification.permission);
  const [chosen, setChosen] = useState<Filter>('all');

  const items = inbox.data ?? [];
  const count = (f: Filter) =>
    f === 'all' ? items.length : items.filter((i) => i.kind === f).length;
  const filter = count(chosen) > 0 ? chosen : 'all';
  const shown = items.filter((i) => filter === 'all' || i.kind === filter);

  return (
    <div className="flex max-w-[760px] flex-col gap-7">
      <header>
        <h1 className="text-4xl tracking-tight">Needs you</h1>
        {items.length > 0 && (
          <p className="mt-1.5 text-muted">
            {items.length === 1 ? '1 thing is waiting.' : `${items.length} things are waiting.`}{' '}
            Everything else is moving on its own.
          </p>
        )}
        {permission === 'default' && (
          <p className="mt-2.5 flex items-center gap-2.5 text-[13px] text-muted">
            Get a Mac notification when something needs you
            <Button
              type="button"
              size="sm"
              onClick={() => void Notification.requestPermission().then(setPermission)}
            >
              Turn on notifications
            </Button>
          </p>
        )}
      </header>
      {inbox.isError && <p className="text-sm text-danger">{inbox.error.message}</p>}
      {items.length > 0 && (
        <div role="group" aria-label="Filter" className="flex flex-wrap gap-1.5">
          {filters
            .filter((f) => f.value === 'all' || count(f.value) > 0)
            .map((f) => (
              <Button
                key={f.value}
                type="button"
                size="sm"
                variant={f.value === filter ? 'primary' : 'secondary'}
                aria-pressed={f.value === filter}
                onClick={() => setChosen(f.value)}
              >
                {f.label} {count(f.value)}
              </Button>
            ))}
        </div>
      )}
      {inbox.data?.length === 0 && (
        <EmptyState title="Nothing needs you" body="Everything is moving on its own.">
          <Creature role="lead" state="asleep" size={64} />
        </EmptyState>
      )}
      <ul className="flex flex-col gap-3">
        {shown.map((item) => (
          <InboxRow
            key={itemKey(item)}
            item={item}
            channel={channels.data?.find((c) => c.id === item.channelId)}
          />
        ))}
      </ul>
    </div>
  );
}
