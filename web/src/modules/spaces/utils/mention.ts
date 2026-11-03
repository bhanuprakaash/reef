export type Mention = { start: number; query: string };

export function findMention(text: string, caret: number): Mention | null {
  const found = /(^|\s)@([\w-]*)$/.exec(text.slice(0, caret));
  return found ? { start: caret - found[2].length - 1, query: found[2] } : null;
}

export function insertMention(text: string, caret: number, mention: Mention, name: string) {
  const head = `${text.slice(0, mention.start)}@${name} `;
  return { text: head + text.slice(caret), caret: head.length };
}

export type MentionOption = { botId: number; name: string; member: boolean };

export function mentionOptions(
  members: { botId: number; name: string }[],
  bots: { id: number; name: string }[],
  query: string,
): MentionOption[] {
  const prefix = query.toLowerCase();
  const starts = (name: string) => name.toLowerCase().startsWith(prefix);
  const matched = members
    .filter((m) => starts(m.name))
    .map((m) => ({ botId: m.botId, name: m.name, member: true }));
  if (query === '' || matched.length > 0) return matched;
  return bots
    .filter((b) => starts(b.name) && !members.some((m) => m.botId === b.id))
    .map((b) => ({ botId: b.id, name: b.name, member: false }));
}
