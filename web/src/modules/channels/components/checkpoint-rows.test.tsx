import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CheckpointRows } from './checkpoint-rows';

const { useTurns, useRollbackTurn } = vi.hoisted(() => ({
  useTurns: vi.fn(),
  useRollbackTurn: vi.fn(),
}));

const turn = { id: 7, channelId: 2, role: 'developer', status: 'done', checkpoint: 'refs/c/7' };

vi.mock('@/modules/channels/api/get-turns', () => ({ useTurns }));
vi.mock('@/modules/channels/api/rollback-turn', () => ({ useRollbackTurn }));

describe('CheckpointRows', () => {
  beforeEach(() => {
    useRollbackTurn.mockReturnValue({ isPending: false, isError: false });
  });

  it('lists the rollbackable turns, newest first, each with a Rollback button', () => {
    useTurns.mockReturnValue({
      data: [turn, { ...turn, id: 9 }],
    });

    const markup = renderToStaticMarkup(<CheckpointRows channelId={2} running={false} />);

    expect(markup).toContain('Checkpoints');
    expect(markup.indexOf('Before turn 9')).toBeLessThan(markup.indexOf('Before turn 7'));
    expect(markup.match(/Rollback/g)).toHaveLength(2);
  });

  it('disables Rollback while a turn runs', () => {
    useTurns.mockReturnValue({ data: [turn] });

    const markup = renderToStaticMarkup(<CheckpointRows channelId={2} running />);

    expect(markup).toContain('disabled');
  });

  it('shows nothing when no turn is rollbackable', () => {
    useTurns.mockReturnValue({ data: [] });

    expect(renderToStaticMarkup(<CheckpointRows channelId={2} running={false} />)).toBe('');
  });
});
