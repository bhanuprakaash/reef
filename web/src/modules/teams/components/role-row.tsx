import type { Role } from '@/types/api';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import { Creature } from '@/components/creature';

const badge = 'inline-flex h-[30px] items-center rounded-pill border px-3 text-xs';

type Props = {
  role: Omit<Role, 'lead'> & { lead?: boolean };
  badges?: string[];
  selected: boolean;
  onSelect: () => void;
};

export function RoleRow({ role, badges, selected, onSelect }: Props) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onSelect}
      className={`flex w-full flex-wrap items-center gap-3.5 rounded-xl border p-3 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring ${selected ? 'border-line bg-bubble' : 'border-transparent hover:bg-card'}`}
    >
      <Creature look={role} size={40} animate />
      <span className="min-w-0 flex-1 basis-40">
        <span className="flex flex-wrap items-center gap-2">
          <span>{role.name || 'role'}</span>
          {badges?.map((text) => (
            <span
              key={text}
              className="rounded-pill border border-line px-2 text-[11px] leading-5 text-muted"
            >
              {text}
            </span>
          ))}
          {role.lead && (
            <span className="rounded-pill bg-lead px-2 text-[11px] leading-5 text-mark-ink">
              answers by default
            </span>
          )}
        </span>
        <span className="block truncate text-[12.5px] text-muted">{role.persona}</span>
      </span>
      <span className={`${badge} border-line text-soft`}>{backendLabel(role.backend)}</span>
      <span
        className={`${badge} ${role.write ? 'border-transparent bg-developer/15 text-developer' : 'border-line text-muted'}`}
      >
        {role.write ? 'edits code' : 'read-only'}
      </span>
      {role.backend === 'claude' && (
        <span className={`${badge} border-line text-muted`}>{role.tools.length} tools</span>
      )}
    </button>
  );
}
