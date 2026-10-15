// parseTools turns the tools textarea's raw text into the tool list, one per line, trimmed
// and with blank lines dropped.
export function parseTools(text: string): string[] {
  return text
    .split('\n')
    .map((t) => t.trim())
    .filter(Boolean);
}
