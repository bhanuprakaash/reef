import { GroupAvatar } from '@/components/creature';
import { StatusDot } from '@/components/status-dot';
import { dotFor, type Chain, type ChainState } from '@/modules/spaces/utils/direct-chains';

type Props = {
  chain: Chain;
  faces: Record<string, string | undefined>;
  open: boolean;
  onOpen: () => void;
};

const live: ChainState[] = ['working', 'paused'];

export function CoordinationItem({ chain, faces, open, onOpen }: Props) {
  const { from, to } = chain.messages[0];
  const count = chain.messages.length;
  const verb = live.includes(chain.state) ? 'are coordinating' : 'coordinated';

  return (
    <li className="relative rounded-card border border-line bg-card hover:bg-hover [--creature-ring:var(--color-card)]">
      <button
        type="button"
        aria-expanded={open}
        onClick={onOpen}
        className="flex w-full items-center gap-3 px-4 py-3 text-left after:absolute after:inset-0 after:rounded-card"
      >
        <GroupAvatar roles={[from, to]} faces={faces} size={24} />
        <span className="min-w-0 flex-1">
          <span className="block text-[14.5px] first-letter:uppercase">
            {`${from} and ${to} ${verb} · ${count} ${count === 1 ? 'message' : 'messages'}`}
          </span>
          <span className="flex items-center gap-1.5 text-[13px] text-muted">
            <StatusDot status={dotFor[chain.state]} />
            {chain.state}
          </span>
        </span>
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="shrink-0 text-muted"
        >
          <path d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </li>
  );
}
