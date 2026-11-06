import { describe, expect, it } from 'vitest';
import type { Provider, Role } from '@/types/api';
import { providerFor, providerOptions } from './providers';

const providers: Provider[] = [
  { name: 'claude', kind: 'claude', detected: true, path: '/usr/bin/claude' },
  { name: 'work', kind: 'claude', detected: false, folder: '/w' },
  { name: 'lab', kind: 'codex', detected: false, folder: '/l' },
];
const role = { name: 'dev', backend: 'claude' } as Role;

describe('providerOptions', () => {
  it('offers PATH and the added installs of the role backend', () => {
    expect(providerOptions(role, providers)).toEqual([
      { value: '', label: 'Default (PATH)' },
      { value: 'work', label: 'work' },
    ]);
  });

  it('keeps a provider that was removed so the select still shows it', () => {
    expect(providerOptions({ ...role, provider: 'gone' }, providers)).toContainEqual({
      value: 'gone',
      label: 'gone',
    });
  });
});

describe('providerFor', () => {
  it('keeps a provider of the new backend and drops the others', () => {
    expect(providerFor('work', 'claude', providers)).toBe('work');
    expect(providerFor('work', 'codex', providers)).toBeUndefined();
    expect(providerFor('gone', 'claude', providers)).toBeUndefined();
  });
});
