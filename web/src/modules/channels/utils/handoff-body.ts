const header = /\[channel \d+, message \d+(?:, thread \d+)?\]\n/;

export function handoffBody(wake: string): string {
  const last = wake.split(header).at(-1) ?? wake;
  return last
    .replace(/^Thread so far:\n[\s\S]*?\n\n(?=\S+ wrote: )/, '')
    .replace(/^\S+ wrote: /, '')
    .trim();
}
