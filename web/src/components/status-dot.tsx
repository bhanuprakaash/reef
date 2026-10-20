const styles = {
  working: 'bg-working',
  'needs-you': 'bg-needs-you',
  paused: 'bg-paused',
  done: 'bg-done',
  failed: 'bg-failed',
  asleep: 'border-[1.5px] border-asleep',
};

export type Status = keyof typeof styles;

export function StatusDot({ status, label }: { status: Status; label?: string }) {
  return (
    <span
      title={label}
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className="relative inline-flex size-2 flex-none items-center justify-center"
    >
      {status === 'working' && (
        <span className="absolute inset-0 rounded-full bg-working motion-safe:animate-ping" />
      )}
      <span className={`relative size-2 rounded-full ${styles[status]}`} />
    </span>
  );
}
