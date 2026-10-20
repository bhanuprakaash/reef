import { Creature } from '@/components/creature';

// WorkingBadge stands in for the team panel when it is closed: the role that is
// working, with a bubble saying so, as a row at the top right of the channel.
export function WorkingBadge({
  role,
  face,
  className = '',
}: {
  role: string;
  face?: string;
  className?: string;
}) {
  return (
    <div
      role="status"
      aria-label={`${role} is working`}
      className={`flex shrink-0 flex-col items-end gap-1 self-end px-3.5 pt-3 md:px-7 ${className}`}
    >
      <span className="rounded-pill bg-card px-3 py-1 text-[12.5px] text-soft shadow-sm">
        working…
      </span>
      <Creature role={role} face={face} state="working" size={36} animate />
    </div>
  );
}
