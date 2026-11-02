export type DateGroup = 'Today' | 'Yesterday' | 'Last 7 days' | 'Older';

export function dateGroup(iso: string, now: Date = new Date()): DateGroup {
  const at = new Date(iso).getTime();
  const midnight = (daysAgo: number) =>
    new Date(now.getFullYear(), now.getMonth(), now.getDate() - daysAgo).getTime();
  if (at >= midnight(0)) return 'Today';
  if (at >= midnight(1)) return 'Yesterday';
  if (at >= midnight(6)) return 'Last 7 days';
  return 'Older';
}
