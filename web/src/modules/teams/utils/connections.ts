import { backendLabel } from '@/modules/teams/utils/backend-label';
import type { AvailableConnection, Connection } from '@/types/api';

export function connectionLabel(c: Connection) {
  if (!c.url) return c.command ?? '';
  try {
    return new URL(c.url).host;
  } catch {
    return c.url;
  }
}

export type SignInStatus = {
  state: 'key' | 'signed_in' | 'needs_sign_in' | 'failed';
  label: string;
  detail?: string;
};

export function signInStatus(c: Connection): SignInStatus | null {
  if (!c.url) return null;
  if (Object.keys(c.headers ?? {}).length > 0) return { state: 'key', label: 'Key header' };
  const entries = c.signIn ?? [];
  const failed = entries.find((e) => e.state === 'failed');
  if (failed) {
    return {
      state: 'failed',
      label: 'Sign-in failed',
      detail: `${backendLabel(failed.backend)}: ${failed.detail}`,
    };
  }
  if (entries.length === 0 || entries.some((e) => e.state === 'needs_sign_in')) {
    return { state: 'needs_sign_in', label: 'Sign in' };
  }
  return { state: 'signed_in', label: 'Signed in' };
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
