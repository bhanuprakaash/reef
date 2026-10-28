import { describe, expect, it } from 'vitest';
import type { Message, Turn } from '@/types/api';
import { checkpointAnchor, unanchoredCheckpoints } from './checkpoint-anchor';
import { groupThreads } from './group-threads';

const message = (id: number, author: string, createdAt: string): Message => ({
  id,
  spaceId: 2,
  parentId: null,
  author,
  body: `body ${id}`,
  createdAt,
});
const turn = (id: number, role: string, startedAt: string): Turn => ({
  id,
  spaceId: 2,
  role,
  status: 'done',
  error: '',
  wakeText: '',
  retried: false,
  checkpoint: `refs/reef/checkpoints/${id}`,
  startedAt,
});

const threads = groupThreads([
  message(1, 'user', '2026-10-18T10:00:00Z'),
  message(2, 'developer', '2026-10-18T10:03:00Z'),
  message(3, 'user', '2026-10-18T11:00:00Z'),
  message(4, 'developer', '2026-10-18T11:05:00Z'),
]);

describe('checkpointAnchor', () => {
  it("anchors a checkpoint to the turn role's last message before the next turn", () => {
    const anchors = checkpointAnchor(
      [turn(7, 'developer', '2026-10-18T10:02:00Z'), turn(9, 'developer', '2026-10-18T11:04:00Z')],
      [turn(7, 'developer', '2026-10-18T10:02:00Z')],
      threads,
    );

    expect(anchors.get(2)?.id).toBe(7);
    expect(anchors.size).toBe(1);
  });

  it('anchors nothing when the role posted nothing in the turn', () => {
    const anchors = checkpointAnchor(
      [turn(7, 'tester', '2026-10-18T10:02:00Z'), turn(9, 'developer', '2026-10-18T11:04:00Z')],
      [turn(7, 'tester', '2026-10-18T10:02:00Z')],
      threads,
    );

    expect(anchors.size).toBe(0);
  });

  it('anchors to the last message when no turn follows', () => {
    const anchors = checkpointAnchor(
      [turn(9, 'developer', '2026-10-18T11:04:00Z')],
      [turn(9, 'developer', '2026-10-18T11:04:00Z')],
      threads,
    );

    expect(anchors.get(4)?.id).toBe(9);
  });
});

describe('unanchoredCheckpoints', () => {
  it('keeps only the checkpoints that anchored to no bubble', () => {
    const seven = turn(7, 'developer', '2026-10-18T10:02:00Z');
    const anchors = checkpointAnchor(
      [seven, turn(9, 'developer', '2026-10-18T11:04:00Z')],
      [seven],
      threads,
    );

    expect(unanchoredCheckpoints([seven], anchors)).toEqual([]);
    expect(
      unanchoredCheckpoints([turn(11, 'developer', '2026-10-18T12:00:00Z')], anchors),
    ).toHaveLength(1);
  });
});
