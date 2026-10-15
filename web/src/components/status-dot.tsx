const styles = {
  working: 'bg-working',
  'needs-you': 'bg-needs-you',
  paused: 'bg-paused',
  done: 'bg-done',
  failed: 'bg-failed',
  asleep: 'border-[1.5px] border-asleep',
};

export type Status = keyof typeof styles;

export function StatusDot({ status }: { status: Status }) {
  return (
    <span aria-hidden="true" className={`inline-block size-2 rounded-full ${styles[status]}`} />
  );
}
