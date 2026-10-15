import type { Team } from '@/types/api';
import { Creature } from '@/components/creature';

type Props = { teams: Team[]; selected: string; onSelect: (name: string) => void };

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

export function TeamList({ teams, selected, onSelect }: Props) {
  return (
    <section aria-label="All teams" className="flex w-full flex-col gap-1.5 lg:w-60">
      {teams.map((t) => {
        const current = t.name === selected;
        const gates = t.gates?.length ?? 0;
        return (
          <div
            key={t.name}
            className={`relative rounded-xl border p-3 ${current ? 'border-line bg-bubble' : 'border-transparent hover:bg-card'}`}
          >
            <button
              type="button"
              aria-current={current || undefined}
              onClick={() => onSelect(t.name)}
              className={`text-sm after:absolute after:inset-0 after:rounded-xl ${current ? 'font-semibold' : 'font-medium text-soft'}`}
            >
              {t.name}
            </button>
            <div className="mt-2.5 flex">
              {t.roles.map((r) => (
                <Creature key={r.name} role={r.name} face={r.face} size={26} />
              ))}
            </div>
            <div className="mt-2 text-xs text-muted">
              {plural(t.roles.length, 'role')}
              {gates > 0 && ` · ${plural(gates, 'gate')}`}
            </div>
          </div>
        );
      })}
    </section>
  );
}
