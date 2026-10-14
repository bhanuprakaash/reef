import { useCallback } from 'react';
import { Link, useNavigate } from 'react-router';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { useInboxEvents } from '@/modules/inbox/hooks/use-inbox-events';

export function InboxLink() {
  const navigate = useNavigate();
  const inbox = useInbox();
  useInboxEvents(useCallback((channelId) => void navigate(`/channels/${channelId}`), [navigate]));

  const count = inbox.data?.length ?? 0;
  return (
    <Link to="/inbox" className="flex items-center gap-2 hover:underline">
      Inbox
      {count > 0 && (
        <span
          aria-label={`${count} waiting`}
          className="rounded-full bg-red-600 px-1.5 text-xs font-medium text-white"
        >
          {count}
        </span>
      )}
    </Link>
  );
}
