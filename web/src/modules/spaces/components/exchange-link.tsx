import { Link } from 'react-router';
import { useExchange } from '@/modules/spaces/api/get-exchange';
import { useSpace } from '@/modules/spaces/api/get-space';
import type { DirectMessage } from '@/types/api';

type Props = { exchangeId: number; spaceId: number };

const linkClass = 'px-1 text-[13px] text-soft underline';

export function ExchangeLink({ exchangeId, spaceId }: Props) {
  const { data } = useExchange(exchangeId);
  if (!data || data.request.spaceId === spaceId) return null;
  if (data.request.kind === 'question' || data.request.kind === 'guidance') {
    return <Receipt request={data.request} />;
  }
  return (
    <Link to={`/spaces/${data.request.spaceId}`} className={linkClass}>
      Open consultation
    </Link>
  );
}

function Receipt({ request }: { request: DirectMessage }) {
  const { data: space } = useSpace(request.spaceId);
  const waiting = space?.paused ? 'Waiting for Space to resume' : 'Queued';
  const state: Record<NonNullable<DirectMessage['state']>, string> = {
    waiting,
    delivered: 'Delivered',
    answered: request.kind === 'question' ? 'Answered' : 'Delivered',
    failed: 'Failed',
    stopped: 'Stopped',
    cancelled: 'Cancelled',
  };
  return (
    <span className="px-1 text-[13px] text-soft">
      {state[request.state ?? 'waiting']} ·{' '}
      <Link to={`/spaces/${request.spaceId}`} className="underline">
        Open Space
      </Link>{' '}
      ·{' '}
      <Link to={`/spaces/${request.spaceId}/bots/${request.toBotId}`} className="underline">
        Activity
      </Link>
    </span>
  );
}
