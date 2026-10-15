import { describe, expect, it } from 'vitest';
import type { Turn } from '@/types/api';
import { checkpointItemLabel, checkpointSummary } from './checkpoint-label';

const turn = (id: number, role = 'developer'): Turn => ({
  id,
  channelId: 2,
  role,
  status: 'done',
  error: '',
  wakeText: '',
  retried: false,
  checkpoint: `refs/c/${id}`,
  startedAt: new Date(2026, 9, 15, 17, 42).toISOString(),
});

describe('checkpointSummary', () => {
  it('names the one checkpoint', () => {
    expect(checkpointSummary([turn(33)])).toBe('Checkpoint before turn 33 by developer');
  });

  it('counts several', () => {
    expect(checkpointSummary([turn(14), turn(16), turn(33)])).toBe('3 checkpoints');
  });
});

describe('checkpointItemLabel', () => {
  it('gives the turn, its role and the local start time', () => {
    expect(checkpointItemLabel(turn(33))).toBe('Before turn 33 · developer · 17:42');
  });

  it('pads the hour', () => {
    const early = { ...turn(4, 'lead'), startedAt: new Date(2026, 9, 15, 7, 5).toISOString() };
    expect(checkpointItemLabel(early)).toBe('Before turn 4 · lead · 07:05');
  });
});
