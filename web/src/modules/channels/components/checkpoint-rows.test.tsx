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

  it('folds several checkpoints into one row with one Roll back… button', () => {
    useTurns.mockReturnValue({
      data: [turn, { ...turn, id: 9 }, { ...turn, id: 12 }],
    });

    const markup = renderToStaticMarkup(<CheckpointRows channelId={2} running={false} />);

    expect(markup).toContain('Checkpoints');
    expect(markup).toContain('3 checkpoints');
    expect(markup).not.toContain('before turn');
    expect(markup.match(/Roll back…/g)).toHaveLength(1);
    expect(markup).toContain('aria-expanded="false"');
  });

  it('names the checkpoint when there is only one', () => {
    useTurns.mockReturnValue({ data: [turn] });

    const markup = renderToStaticMarkup(<CheckpointRows channelId={2} running={false} />);

    expect(markup).toContain('Checkpoint before turn 7 by developer');
    expect(markup).not.toContain('checkpoints');
  });

  it('disables Roll back… while a turn runs', () => {
    useTurns.mockReturnValue({ data: [turn] });

    const markup = renderToStaticMarkup(<CheckpointRows channelId={2} running />);

    expect(markup).toContain('disabled');
  });

  it('shows nothing when no turn is rollbackable', () => {
    useTurns.mockReturnValue({ data: [] });

    expect(renderToStaticMarkup(<CheckpointRows channelId={2} running={false} />)).toBe('');
  });
});
