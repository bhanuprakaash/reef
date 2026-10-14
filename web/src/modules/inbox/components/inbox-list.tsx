import { useState } from 'react';
import { useChannels } from '@/modules/inbox/api/get-channels';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { InboxRow } from '@/modules/inbox/components/inbox-row';
import { itemKey } from '@/modules/inbox/utils/new-items';
import { buttonClass, errorClass, mutedClass } from '@/utils/classes';

export function InboxList() {
  const inbox = useInbox();
  const channels = useChannels();
  const [permission, setPermission] = useState(Notification.permission);

  return (
    <div className="max-w-3xl space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">Inbox</h1>
        {permission === 'default' && (
          <button
            type="button"
            className={buttonClass}
            onClick={() => void Notification.requestPermission().then(setPermission)}
          >
            Turn on notifications
          </button>
        )}
      </div>
      {inbox.isError && <p className={errorClass}>{inbox.error.message}</p>}
      {inbox.data?.length === 0 && <p className={mutedClass}>Nothing waits on you.</p>}
      <ul className="space-y-2">
        {inbox.data?.map((item) => (
          <InboxRow
            key={itemKey(item)}
            item={item}
            channelName={
              channels.data?.find((c) => c.id === item.channelId)?.name ??
              `Channel ${item.channelId}`
            }
          />
        ))}
      </ul>
    </div>
  );
}
