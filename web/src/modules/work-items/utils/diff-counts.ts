import type { FileDiff } from '@/types/api';

export function diffCounts(files: FileDiff[]) {
  const lines = files.flatMap((f) => f.hunks.flatMap((h) => h.lines));
  return {
    added: lines.filter((l) => l.kind === 'add').length,
    removed: lines.filter((l) => l.kind === 'del').length,
  };
}
