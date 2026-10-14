import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { FailedTurnRows } from './failed-turn-rows';

const { useFailedTurns, useRetryTurn } = vi.hoisted(() => ({
  useFailedTurns: vi.fn(),
  useRetryTurn: vi.fn(),
}));

const turn = {
  id: 7,
  channelId: 2,
  role: 'developer',
  status: 'failed',
  error: 'server restarted',
  wakeText: 'build it',
  retried: false,
};

vi.mock('@/modules/channels/api/get-failed-turns', () => ({ useFailedTurns }));
vi.mock('@/lib/api/retry-turn', () => ({ useRetryTurn }));

describe('FailedTurnRows', () => {
  beforeEach(() => {
    useRetryTurn.mockReturnValue({ isPending: false, isError: false });
  });

  it('shows a failed turn with a Retry button', () => {
    useFailedTurns.mockReturnValue({ data: [turn] });

    const markup = renderToStaticMarkup(<FailedTurnRows channelId={2} running={false} />);

    expect(markup).toContain('Turn 7 by developer failed: server restarted');
    expect(markup).toContain('Retry');
  });

  it('shows nothing when no turn can be retried', () => {
    useFailedTurns.mockReturnValue({ data: [] });

    const markup = renderToStaticMarkup(<FailedTurnRows channelId={2} running={false} />);

    expect(markup).toBe('');
  });
});
