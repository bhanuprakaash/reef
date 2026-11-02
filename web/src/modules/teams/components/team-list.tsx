import type { Team } from '@/types/api';
import { Creature } from '@/components/creature';

type Props = { teams: Team[]; selected: string; onSelect: (name: string) => void };

export function TeamList({ teams, selected, onSelect }: Props) {
  return (
    <nav aria-label="All crews" className="flex w-full flex-col gap-0.5 md:w-52 md:shrink-0">
      {teams.map((t) => {
        const current = t.name === selected;
        return (
          <button
            key={t.name}
            type="button"
            aria-current={current || undefined}
            onClick={() => onSelect(t.name)}
            className={`flex h-12 items-center gap-2.5 rounded-xl px-2.5 text-left text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring md:h-[46px] ${current ? 'bg-active text-fg' : 'text-soft hover:bg-hover/50'}`}
          >
            <span className="flex w-[58px] shrink-0">
              {t.roles.slice(0, 3).map((r, i) => (
                <span key={r.botId} className={`rounded-full bg-main ${i ? '-ml-1.5' : ''}`}>
                  <Creature look={r} size={22} />
                </span>
              ))}
            </span>
            <span className="min-w-0 flex-1 truncate">{t.name}</span>
            {t.name === 'solo' ? (
              <span className="rounded-pill border border-line px-1.5 text-[10.5px] leading-4 text-muted">
                default
              </span>
            ) : (
              <span className="text-[11.5px] text-faint">{t.roles.length}</span>
            )}
          </button>
        );
      })}
    </nav>
  );
}
