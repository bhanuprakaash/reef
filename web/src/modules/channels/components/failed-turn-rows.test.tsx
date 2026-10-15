import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { FailedTurnRows } from './failed-turn-rows';

const { useFailedTurns, useRetryTurn, useTurns, useRollbackTurn } = vi.hoisted(() => ({
  useFailedTurns: vi.fn(),
  useRetryTurn: vi.fn(),
  useTurns: vi.fn(),
  useRollbackTurn: vi.fn(),
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
vi.mock('@/modules/channels/api/get-turns', () => ({ useTurns }));
vi.mock('@/modules/channels/api/rollback-turn', () => ({ useRollbackTurn }));

describe('FailedTurnRows', () => {
  beforeEach(() => {
    useRetryTurn.mockReturnValue({ isPending: false, isError: false });
    useRollbackTurn.mockReturnValue({ isPending: false, isError: false });
    useTurns.mockReturnValue({ data: [] });
  });

  it('shows a failed turn with a Retry button', () => {
    useFailedTurns.mockReturnValue({ data: [turn] });

    const markup = renderToStaticMarkup(
      <FailedTurnRows channelId={2} running={false} paused={false} />,
    );

    expect(markup).toContain('>developer</span>&#x27;s turn 7 failed');
    expect(markup).toContain('server restarted');
    expect(markup).toContain('Retry');
  });

  it('hides Retry while the channel is paused', () => {
    useFailedTurns.mockReturnValue({ data: [turn] });

    const markup = renderToStaticMarkup(
      <FailedTurnRows channelId={2} running={false} paused={true} />,
    );

    expect(markup).toContain('>developer</span>&#x27;s turn 7 failed');
    expect(markup).toContain('server restarted');
    expect(markup).not.toContain('Retry');
  });

  it('offers Roll back only for a failed turn that has a checkpoint', () => {
    useFailedTurns.mockReturnValue({ data: [turn, { ...turn, id: 8 }] });
    useTurns.mockReturnValue({ data: [{ id: 8 }] });

    const markup = renderToStaticMarkup(
      <FailedTurnRows channelId={2} running={false} paused={false} />,
    );

    expect(markup.match(/Roll back/g)).toHaveLength(1);
  });

  it('shows nothing when no turn can be retried', () => {
    useFailedTurns.mockReturnValue({ data: [] });

    const markup = renderToStaticMarkup(
      <FailedTurnRows channelId={2} running={false} paused={false} />,
    );

    expect(markup).toBe('');
  });
});
