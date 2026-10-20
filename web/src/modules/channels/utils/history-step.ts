export function stepHistory(
  history: string[],
  recalled: number | null,
  text: string,
  key: 'ArrowUp' | 'ArrowDown',
): { index: number | null; text: string } | null {
  const walking = recalled !== null && history[recalled] === text;
  if (key === 'ArrowUp') {
    if (!walking && text !== '') return null;
    const index = walking ? Math.min(recalled + 1, history.length - 1) : 0;
    return history.length === 0 ? null : { index, text: history[index] };
  }
  if (!walking) return null;
  return recalled === 0
    ? { index: null, text: '' }
    : { index: recalled - 1, text: history[recalled - 1] };
}
