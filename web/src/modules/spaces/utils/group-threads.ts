import type { Message } from '@/types/api';

export type Thread = { root: Message; replies: Message[] };

export function groupThreads(messages: Message[]): Thread[] {
  const roots = new Map<number, Thread>();
  const threads: Thread[] = [];
  for (const m of messages) {
    const thread = m.parentId === null ? undefined : roots.get(m.parentId);
    if (thread) {
      thread.replies.push(m);
    } else {
      const t: Thread = { root: m, replies: [] };
      roots.set(m.id, t);
      threads.push(t);
    }
  }
  return threads;
}
