const header =
  /\[channel \d+, (?:direct )?message \d+(?:, (?:thread|chain) \d+)?(?:, source: [^\]]+)?\]\n/;

export function handoffBody(wake: string): string {
  const last = wake.split(header).at(-1) ?? wake;
  return last
    .replace(/^Asked together: [^\n]*\n/, '')
    .replace(/^\S+ wrote(?: privately)?: /, '')
    .trim();
}
