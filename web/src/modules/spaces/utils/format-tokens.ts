export function formatTokens(n: number) {
  return n < 1000 ? String(n) : `${(n / 1000).toFixed(1)}k`;
}
