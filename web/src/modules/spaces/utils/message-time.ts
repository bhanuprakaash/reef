const _tenMinutes = 10 * 60 * 1000;

export function showTimestamp(previous: string | undefined, current: string): boolean {
  return previous === undefined || Date.parse(current) - Date.parse(previous) > _tenMinutes;
}

export function timestampLabel(current: string, previous: string | undefined, now: Date): string {
  const at = new Date(current);
  const time = at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (previous !== undefined && new Date(previous).toDateString() === at.toDateString()) {
    return time;
  }
  const day =
    at.toDateString() === now.toDateString()
      ? 'Today'
      : at.toLocaleDateString([], { month: 'short', day: 'numeric' });
  return `${day} ${time}`;
}
