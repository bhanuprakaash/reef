import type { Team } from '@/types/api';
import { buttonClass } from '@/utils/classes';

type Props = { teams: Team[]; selected: string; onSelect: (name: string) => void };

export function TeamList({ teams, selected, onSelect }: Props) {
  return (
    <ul className="flex flex-wrap gap-2">
      {teams.map((t) => (
        <li key={t.name}>
          <button
            type="button"
            className={`${buttonClass} ${t.name === selected ? 'bg-neutral-100 dark:bg-neutral-800' : ''}`}
            onClick={() => onSelect(t.name)}
          >
            {t.name}
          </button>
        </li>
      ))}
    </ul>
  );
}
