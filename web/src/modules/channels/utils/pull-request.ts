import type { Message } from '@/types/api';

export type PullRequest = { number: number; state: string; url: string };

const NOTE = /^PR #(\d+) (\w+): (\S+)$/;

export function latestPullRequest(messages: Message[]): PullRequest | null {
  for (const m of [...messages].reverse()) {
    const match = m.author === 'reef' ? NOTE.exec(m.body) : null;
    if (match) return { number: Number(match[1]), state: match[2], url: match[3] };
  }
  return null;
}
