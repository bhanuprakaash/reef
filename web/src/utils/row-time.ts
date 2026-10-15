const DAY = 24 * 60 * 60 * 1000;

export function rowTime(iso: string, now: Date = new Date()): string {
  const at = new Date(iso);
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (at.getTime() >= midnight) {
    return at.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: false });
  }
  if (at.getTime() >= midnight - 6 * DAY) {
    return at.toLocaleDateString('en-US', { weekday: 'short' });
  }
  return at.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}
