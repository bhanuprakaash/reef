import type { Team } from '@/types/api';
import { Button } from '@/components/button';

type Props = { teams: Team[]; selected: string; onSelect: (name: string) => void };

export function TeamList({ teams, selected, onSelect }: Props) {
  return (
    <ul className="flex flex-wrap gap-2">
      {teams.map((t) => (
        <li key={t.name}>
          <Button
            type="button"
            variant={t.name === selected ? 'primary' : 'secondary'}
            onClick={() => onSelect(t.name)}
          >
            {t.name}
          </Button>
        </li>
      ))}
    </ul>
  );
}
