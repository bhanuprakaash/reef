import { expect, it } from 'vitest';
import { pairsText, parsePairs, signInStatus } from './connections';

it('shows header values masked and sends the mask back unchanged', () => {
  const text = pairsText({ Authorization: '****', 'X-Team': '****' }, ': ');

  expect(text).toBe('Authorization: ****\nX-Team: ****');
  expect(parsePairs(text, ': ')).toEqual({ Authorization: '****', 'X-Team': '****' });
});

it('keeps a colon inside a header value', () => {
  expect(parsePairs('Referer: https://a.example:8080/x', ': ')).toEqual({
    Referer: 'https://a.example:8080/x',
  });
});

it('parses environment lines on the first =', () => {
  expect(parsePairs('TOKEN=a=b\nbad line\nHOME=/x')).toEqual({ TOKEN: 'a=b', HOME: '/x' });
});

const signIn = (backend: string, state: 'signed_in' | 'needs_sign_in' | 'failed', detail = '') => ({
  backend,
  state,
  detail,
  updatedAt: '2026-11-02T10:00:00Z',
});
const remote = { id: 1, name: 'gitlab', url: 'https://gitlab.com/api/v4/mcp', createdAt: '' };

it('labels a header connection Key header and a command connection with no state', () => {
  expect(signInStatus({ ...remote, headers: { Authorization: '****' } })).toEqual({
    state: 'key',
    label: 'Key header',
  });
  expect(signInStatus({ id: 2, name: 'files', command: 'npx', createdAt: '' })).toBeNull();
});

it('labels a URL connection by its worst sign-in state', () => {
  const both = (a: 'signed_in' | 'needs_sign_in', b: 'signed_in' | 'needs_sign_in') => ({
    ...remote,
    signIn: [signIn('claude', a), signIn('codex', b)],
  });

  expect(signInStatus(remote)?.label).toBe('Sign in');
  expect(signInStatus({ ...remote, signIn: [] })?.label).toBe('Sign in');
  expect(signInStatus(both('signed_in', 'needs_sign_in'))?.label).toBe('Sign in');
  expect(signInStatus(both('signed_in', 'signed_in'))?.label).toBe('Signed in');
});

it('shows a failed sign-in with the backend and its detail', () => {
  expect(
    signInStatus({
      ...remote,
      signIn: [signIn('claude', 'signed_in'), signIn('codex', 'failed', 'login timed out')],
    }),
  ).toEqual({ state: 'failed', label: 'Sign-in failed', detail: 'Codex: login timed out' });
});
