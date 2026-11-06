export function preview(text: string): string {
  return text
    .split('\n')
    .map((line) =>
      line
        .replace(/^\s*(?:#{1,6}\s+|>\s?|[-*+]\s+|\d+[.)]\s+|```\w*)/, '')
        .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
        .replace(/(\*\*|__|\*|`)(.+?)\1/g, '$2')
        .trim(),
    )
    .filter(Boolean)
    .join(' · ');
}
