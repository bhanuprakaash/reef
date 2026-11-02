import type { AvailableConnection, Connection } from '@/types/api';

export function connectionLabel(c: Connection) {
  if (!c.url) return c.command ?? '';
  try {
    return new URL(c.url).host;
  } catch {
    return c.url;
  }
}

export function availableLabel(a: AvailableConnection) {
  return a.kind === 'url' ? (a.url ?? '') : [a.command, ...(a.args ?? [])].join(' ');
}

export function pairsText(pairs?: Record<string, string>, sep = '=') {
  return Object.entries(pairs ?? {})
    .map(([k, v]) => `${k}${sep}${v}`)
    .join('\n');
}

export function parsePairs(text: string, sep = '=') {
  const pairs: Record<string, string> = {};
  for (const line of text.split('\n')) {
    const at = line.indexOf(sep);
    if (at > 0) pairs[line.slice(0, at).trim()] = line.slice(at + sep.length);
  }
  return pairs;
}
