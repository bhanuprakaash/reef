import { Link } from 'react-router';
import { useExchangeAction } from '@/modules/spaces/api/exchange-action';
import { useExchange } from '@/modules/spaces/api/get-exchange';
import { useSpace } from '@/modules/spaces/api/get-space';
import { Button } from '@/components/button';
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

export function exchangeState(request: DirectMessage, paused: boolean) {
  const state: Record<NonNullable<DirectMessage['state']>, string> = {
    waiting: paused ? 'Waiting for Space to resume' : 'Queued — after the current turn',
    delivered: 'Delivered',
    answered: request.kind === 'question' ? 'Answered' : 'Delivered',
    failed: 'Failed',
    stopped: 'Stopped',
    cancelled: 'Cancelled',
  };
  return state[request.state ?? 'waiting'];
}

export function ExchangeActions({ request }: { request: DirectMessage }) {
  const retry = useExchangeAction('retry', request.id, request.spaceId);
  const cancel = useExchangeAction('cancel', request.id, request.spaceId);
  if (request.state === 'failed' || request.state === 'stopped') {
    return (
      <Button
        size="sm"
        variant="secondary"
        disabled={retry.isPending}
        onClick={() => retry.mutate()}
      >
        Retry
      </Button>
    );
  }
  if (request.state === 'waiting') {
    return (
      <Button size="sm" variant="ghost" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
        Cancel
      </Button>
    );
  }
  return null;
}

function Receipt({ request }: { request: DirectMessage }) {
  const { data: space } = useSpace(request.spaceId);
  return (
    <span className="px-1 text-[13px] text-soft">
      {exchangeState(request, !!space?.paused)} ·{' '}
      <Link to={`/spaces/${request.spaceId}`} className="underline">
        Open Space
      </Link>{' '}
      ·{' '}
      <Link to={`/spaces/${request.spaceId}/bots/${request.toBotId}`} className="underline">
        Activity
      </Link>{' '}
      <ExchangeActions request={request} />
    </span>
  );
}
