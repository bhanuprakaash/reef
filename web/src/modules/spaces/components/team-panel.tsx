import { Link } from 'react-router';
import { useInspector } from '@/modules/spaces/api/get-inspector';
import { formatTokens } from '@/modules/spaces/utils/format-tokens';
import type { Role } from '@/types/api';
import { Chip } from '@/components/chip';
import { Markdown } from '@/components/markdown';
import { Creature, type CreatureState } from '@/components/creature';

type Props = {
  spaceId: number;
  workItemId: number | null;
  roles: Role[];
  working: string | null;
  brief?: string;
  onClose: () => void;
};

const backendLabel: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

const stateLabel: Record<CreatureState, string> = {
  working: 'working',
  'needs-you': 'waiting on you',
  asleep: 'asleep',
  idle: 'idle',
  failed: 'failed',
};

export function TeamPanel({ spaceId, workItemId, roles, working, brief, onClose }: Props) {
  const inspector = useInspector(spaceId, true);

  function stateOf(role: Role): CreatureState {
    if (role.name === working) return 'working';
    return role.lead ? 'idle' : 'asleep';
  }

  return (
    <aside aria-label="Team" className="space-y-6">
      <div>
        <div className="mb-2 flex items-center">
          <h2 className="text-[13px] text-muted">Team</h2>
          <button
            type="button"
            aria-label="Close team"
            onClick={onClose}
            className="ml-auto flex size-8 items-center justify-center rounded-field text-soft hover:bg-card max-md:hidden"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M6 6l12 12M18 6L6 18" />
            </svg>
          </button>
        </div>
        <ul className="space-y-0.5">
          {roles.map((role) => {
            const state = stateOf(role);
            const usage = inspector.data?.roles.find((r) => r.role === role.name)?.usage;
            return (
              <li
                key={role.name}
                className={`flex items-center gap-3 rounded-field px-1.5 py-2 ${state === 'working' ? 'bg-card' : ''}`}
              >
                <Creature look={role} state={state} size={30} animate />
                <span className="min-w-0 flex-1">
                  <span
                    className={`flex items-center gap-2 text-[14.5px] ${state === 'asleep' ? 'text-muted' : ''}`}
                  >
                    <Link to={`/bots?bot=${role.botId}`} className="truncate capitalize">
                      {role.name}
                    </Link>
                    {role.lead && <Chip className="h-5 px-2 text-[11px]">lead</Chip>}
                  </span>
                  <span className="block text-[12.5px] text-muted">
                    {[backendLabel[role.backend] ?? role.backend, stateLabel[state]]
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                </span>
                {usage && (
                  <span className="text-[12.5px] tabular-nums text-muted">
                    {formatTokens(usage.inputTokens + usage.outputTokens)} tokens
                  </span>
                )}
              </li>
            );
          })}
        </ul>
      </div>
      {workItemId !== null && (
        <div>
          <h2 className="mb-1.5 text-[13px] text-muted">Brief</h2>
          <div className="text-[13.5px] text-soft">
            <Markdown>{brief || 'No brief yet.'}</Markdown>
          </div>
        </div>
      )}
    </aside>
  );
}
