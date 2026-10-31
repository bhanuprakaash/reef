import type { Connection } from '@/types/api';

export function connectionLabel(c: Connection) {
  if (!c.url) return c.command ?? '';
  try {
    return new URL(c.url).host;
  } catch {
    return c.url;
  }
}
