import type { Turn } from '@/types/api';
import type { Thread } from '@/modules/spaces/utils/group-threads';

// The bubble a checkpoint's rollback icon hangs on: the last top-level message
// the checkpointed turn's own Bot posted. Turns of a Space run serially, so
// the turn's output is what sits between its start and the next turn's. A
// checkpoint whose Bot posted no such message anchors nowhere.
export function checkpointAnchor(
  turns: Turn[],
  checkpoints: Turn[],
  threads: Thread[],
): Map<number, Turn> {
  const started = turns.map((t) => Date.parse(t.startedAt)).sort((a, b) => a - b);
  const anchors = new Map<number, Turn>();
  for (const c of checkpoints) {
    const start = Date.parse(c.startedAt);
    const next = started.find((at) => at > start);
    let anchor: Thread['root'] | undefined;
    for (const { root } of threads) {
      const at = Date.parse(root.createdAt);
      if (
        root.botId != null &&
        root.botId === c.botId &&
        at >= start &&
        (next === undefined || at < next)
      ) {
        anchor = root;
      }
    }
    if (anchor) anchors.set(anchor.id, c);
  }
  return anchors;
}

// Checkpoints that anchored to no bubble. They keep the fallback row at the
// bottom of the conversation so a rollback never becomes unreachable.
export function unanchoredCheckpoints(checkpoints: Turn[], anchors: Map<number, Turn>): Turn[] {
  const anchored = new Set(anchors.values());
  return checkpoints.filter((t) => !anchored.has(t));
}
