export type Piece = { value: string; mention: boolean };

const handoff = /^[ \t]*((?:@[\w-]+[ \t,]+)*@[\w-]+)/gm;

export function splitMentions(text: string): Piece[] {
  const pieces: Piece[] = [];
  let last = 0;
  for (const found of text.matchAll(handoff)) {
    const run = found[1];
    const runStart = found.index + found[0].length - run.length;
    for (const name of run.matchAll(/@[\w-]+/g)) {
      const start = runStart + name.index;
      if (start > last) pieces.push({ value: text.slice(last, start), mention: false });
      pieces.push({ value: name[0], mention: true });
      last = start + name[0].length;
    }
  }
  if (last < text.length) pieces.push({ value: text.slice(last), mention: false });
  return pieces;
}
