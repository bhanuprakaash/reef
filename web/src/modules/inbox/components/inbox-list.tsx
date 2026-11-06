import { Button } from '@/components/button';
import { EmptyState } from '@/components/empty-state';
import { usePendingMemory } from '@/lib/api/get-pending-memory';
import { useSpaces } from '@/lib/api/get-spaces';
import { useNotificationPermission } from '@/lib/use-notification-permission';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { PendingMemoryList } from '@/modules/inbox/components/pending-memory';
import { InboxRow } from '@/modules/inbox/components/inbox-row';
import { useDismissItems } from '@/modules/inbox/api/dismiss-item';
import { useSeenItems } from '@/modules/inbox/hooks/use-seen-items';
import { dismissableSeen, itemKey } from '@/modules/inbox/utils/new-items';
import type { InboxItem } from '@/types/api';

const groups: { group: InboxItem['group']; title: string; hint: string }[] = [
  { group: 'needs_you', title: 'Needs you', hint: 'Work is stopped until you act.' },
  { group: 'fyi', title: 'FYI', hint: 'Nothing is blocked.' },
];

export function InboxList() {
  const inbox = useInbox();
  const spaces = useSpaces();
  const pendingMemory = usePendingMemory();
  const { permission, request } = useNotificationPermission();
  const { seen, markSeen } = useSeenItems();
  const dismissAll = useDismissItems();
  const items = inbox.data ?? [];
  const needsYou = items.filter((i) => i.group === 'needs_you').length;
  const seenFyi = dismissableSeen(items, seen);

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-7 py-6">
      <header>
        <h1 className="text-4xl tracking-tight">Inbox</h1>
        {items.length > 0 && (
          <p className="mt-1.5 text-muted">
            {items.length === 1 ? '1 thing is waiting.' : `${items.length} things are waiting.`}{' '}
            {needsYou === 1 ? '1 needs you.' : `${needsYou} need you.`}
          </p>
        )}
        {permission === 'default' && (
          <p className="mt-2.5 flex items-center gap-2.5 text-[13px] text-muted">
            Get a Mac notification when something needs you
            <Button type="button" size="sm" onClick={request}>
              Turn on notifications
            </Button>
          </p>
        )}
      </header>
      {inbox.isError && <p className="text-sm text-danger">{inbox.error.message}</p>}
      {dismissAll.isError && <p className="text-sm text-danger">{dismissAll.error.message}</p>}
      <PendingMemoryList />
      {inbox.data?.length === 0 && !pendingMemory.data?.length && (
        <EmptyState
          title="Nothing needs you"
          body="Everything is moving on its own."
          face="jellyfish"
        />
      )}
      {groups.map(({ group, title, hint }) => {
        const shown = items.filter((i) => i.group === group);
        if (shown.length === 0) return null;
        return (
          <section key={group} aria-labelledby={`inbox-${group}`} className="flex flex-col gap-1">
            <div className="flex items-baseline gap-2.5">
              <h2 id={`inbox-${group}`} className="text-[15px]">
                {title} <span className="font-normal text-muted">· {shown.length}</span>
              </h2>
              <p className="text-[13px] text-muted">{hint}</p>
              {group === 'fyi' && seenFyi.length > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="ml-auto"
                  disabled={dismissAll.isPending}
                  onClick={() => dismissAll.mutate(seenFyi)}
                >
                  Dismiss all seen ({seenFyi.length})
                </Button>
              )}
            </div>
            <ul className="flex flex-col border-t border-hairline">
              {shown.map((item) => (
                <InboxRow
                  key={itemKey(item)}
                  item={item}
                  space={spaces.data?.find((c) => c.id === item.spaceId)}
                  seen={seen.includes(itemKey(item))}
                  onSeen={() => markSeen(itemKey(item))}
                />
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}
