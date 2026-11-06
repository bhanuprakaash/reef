import type { Mention } from '@/modules/spaces/utils/mention';

export function findSpaceRef(text: string, caret: number): Mention | null {
  const found = /(^|\s)#([^\s#[\]]*)$/.exec(text.slice(0, caret));
  return found ? { start: caret - found[2].length - 1, query: found[2] } : null;
}

export function insertSpaceRef(
  text: string,
  caret: number,
  ref: Mention,
  space: { id: number; name: string },
) {
  const name = space.name.replace(/[[\]\n]/g, ' ');
  const head = `${text.slice(0, ref.start)}#[${name}](space:${space.id}) `;
  return { text: head + text.slice(caret), caret: head.length };
}
