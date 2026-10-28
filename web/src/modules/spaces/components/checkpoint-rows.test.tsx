import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Turn } from '@/types/api';
import { CheckpointRows } from './checkpoint-rows';

const { useRollbackTurn } = vi.hoisted(() => ({
  useRollbackTurn: vi.fn(),
}));

const turn: Turn = {
  id: 7,
  spaceId: 2,
  role: 'developer',
  status: 'done',
  error: '',
  wakeText: '',
  retried: false,
  checkpoint: 'refs/c/7',
  startedAt: '2026-10-18T10:02:00Z',
};

vi.mock('@/modules/spaces/api/rollback-turn', () => ({ useRollbackTurn }));

describe('CheckpointRows', () => {
  beforeEach(() => {
    useRollbackTurn.mockReturnValue({ isPending: false, isError: false });
  });

  it('folds several checkpoints into one row with one Roll back… button', () => {
    const markup = renderToStaticMarkup(
      <CheckpointRows
        spaceId={2}
        running={false}
        turns={[turn, { ...turn, id: 9 }, { ...turn, id: 12 }]}
      />,
    );

    expect(markup).toContain('Checkpoints');
    expect(markup).toContain('3 checkpoints');
    expect(markup).not.toContain('before turn');
    expect(markup.match(/Roll back…/g)).toHaveLength(1);
    expect(markup).toContain('aria-expanded="false"');
  });

  it('names the checkpoint when there is only one', () => {
    const markup = renderToStaticMarkup(
      <CheckpointRows spaceId={2} running={false} turns={[turn]} />,
    );

    expect(markup).toContain('Checkpoint before turn 7 by developer');
    expect(markup).not.toContain('checkpoints');
  });

  it('disables Roll back… while a turn runs', () => {
    const markup = renderToStaticMarkup(<CheckpointRows spaceId={2} running turns={[turn]} />);

    expect(markup).toContain('disabled');
  });

  it('shows nothing when no checkpoint anchored to a bubble', () => {
    expect(renderToStaticMarkup(<CheckpointRows spaceId={2} running={false} turns={[]} />)).toBe(
      '',
    );
  });
});
