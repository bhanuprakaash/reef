import { useCallback } from 'react';
import { Link, useNavigate } from 'react-router';
import { CountBadge } from '@/components/count-badge';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { useInboxEvents } from '@/modules/inbox/hooks/use-inbox-events';
import { itemPath } from '@/utils/anchors';

export function InboxLink() {
  const navigate = useNavigate();
  const inbox = useInbox();
  useInboxEvents(useCallback((item) => void navigate(itemPath(item)), [navigate]));

  const count = inbox.data?.length ?? 0;
  return (
    <Link to="/inbox" className="flex items-center gap-2 hover:underline">
      Inbox
      {count > 0 && <CountBadge count={count} label={`${count} waiting`} />}
    </Link>
  );
}
