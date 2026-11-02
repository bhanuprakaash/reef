import { useState } from 'react';

const KEY = 'reef.inbox.seen';
const KEPT = 200;

export function useSeenItems() {
  const [seen, setSeen] = useState<string[]>(() => {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(KEY) ?? '[]');
      return Array.isArray(stored) ? stored : [];
    } catch {
      return [];
    }
  });
  const markSeen = (key: string) => {
    if (seen.includes(key)) return;
    const next = [...seen, key].slice(-KEPT);
    setSeen(next);
    try {
      localStorage.setItem(KEY, JSON.stringify(next));
    } catch {
      // storage can be blocked; the dots then last until reload
    }
  };
  return { seen, markSeen };
}
