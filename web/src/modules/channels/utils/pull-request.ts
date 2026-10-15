import type { Message } from '@/types/api';

export type PullRequest = { number: number; state: string; url: string };

const NOTE = /^PR #(\d+) (\w+): (\S+)$/;
const CHECK = /^[^\n]+ (succeeded|failed)$/;

export function pullRequestNote(m: Message): PullRequest | null {
  const match = m.author === 'reef' ? NOTE.exec(m.body) : null;
  return match ? { number: Number(match[1]), state: match[2], url: match[3] } : null;
}

export function checkResult(m: Message): 'succeeded' | 'failed' | null {
  const match = m.author === 'reef' ? CHECK.exec(m.body) : null;
  return match ? (match[1] as 'succeeded' | 'failed') : null;
}

export function latestPullRequest(messages: Message[]): PullRequest | null {
  for (const m of [...messages].reverse()) {
    const pr = pullRequestNote(m);
    if (pr) return pr;
  }
  return null;
}
