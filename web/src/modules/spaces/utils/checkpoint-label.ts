import type { Turn } from '@/types/api';

export function checkpointSummary(turns: Turn[]): string {
  if (turns.length === 1) {
    return `Checkpoint before turn ${turns[0].id} by ${turns[0].role}`;
  }
  return `${turns.length} checkpoints`;
}

export function checkpointItemLabel(turn: Turn): string {
  const time = new Date(turn.startedAt).toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
  return `Before turn ${turn.id} · ${turn.role} · ${time}`;
}
