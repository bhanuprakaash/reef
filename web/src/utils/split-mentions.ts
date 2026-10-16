export type Piece = { value: string; mention: boolean };

export function splitMentions(text: string): Piece[] {
  const pieces: Piece[] = [];
  let last = 0;
  for (const found of text.matchAll(/(^|\s)(@[\w-]+)/g)) {
    const start = found.index + found[1].length;
    if (start > last) pieces.push({ value: text.slice(last, start), mention: false });
    pieces.push({ value: found[2], mention: true });
    last = start + found[2].length;
  }
  if (last < text.length) pieces.push({ value: text.slice(last), mention: false });
  return pieces;
}
