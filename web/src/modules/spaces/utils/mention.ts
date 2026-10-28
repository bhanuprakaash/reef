export type Mention = { start: number; query: string };

export function findMention(text: string, caret: number): Mention | null {
  const found = /(^|\s)@([\w-]*)$/.exec(text.slice(0, caret));
  return found ? { start: caret - found[2].length - 1, query: found[2] } : null;
}

export function insertMention(text: string, caret: number, mention: Mention, name: string) {
  const head = `${text.slice(0, mention.start)}@${name} `;
  return { text: head + text.slice(caret), caret: head.length };
}
