import type { Bot } from '@/types/api';

export function memberFaces(bots: Bot[] | undefined, members: number[] | undefined) {
  return Object.fromEntries(
    (members ?? []).flatMap((id) => bots?.find((b) => b.id === id) ?? []).map((b) => [b.id, b]),
  );
}
