const _maxLength = 80;

export function summarizeInput(input: unknown): string {
  let text = '';
  if (typeof input === 'object' && input !== null) {
    const first = Object.values(input).find((v) => typeof v === 'string');
    text = first ?? JSON.stringify(input);
  } else if (input !== undefined) {
    text = String(input);
  }
  text = text.replace(/\s+/g, ' ').trim();
  return text.length > _maxLength ? `${text.slice(0, _maxLength)}...` : text;
}
