import type { Provider, Role } from '@/types/api';

export function providerOptions(role: Role, providers: Provider[]) {
  const installs = providers.filter((p) => !p.detected && p.kind === role.backend);
  const values = ['', ...installs.map((p) => p.name)];
  if (role.provider && !values.includes(role.provider)) values.push(role.provider);
  return values.map((value) => ({ value, label: value || 'Default (PATH)' }));
}

export function providerFor(provider: string | undefined, backend: string, providers: Provider[]) {
  return providers.some((p) => p.name === provider && p.kind === backend) ? provider : undefined;
}
