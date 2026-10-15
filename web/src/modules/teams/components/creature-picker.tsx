import { Creature, creatureNames } from '@/components/creature';

type Props = { value: string; onChange: (face: string) => void };

const label = (name: string) => `${name[0].toUpperCase()}${name.slice(1).replace('-', ' ')}`;

export function CreaturePicker({ value, onChange }: Props) {
  return (
    <div className="flex flex-wrap gap-2.5">
      {creatureNames.map((name) => (
        <button
          key={name}
          type="button"
          aria-label={`Creature: ${label(name)}`}
          aria-pressed={name === value}
          onClick={() => onChange(name)}
          className={`flex size-14 items-center justify-center rounded-2xl bg-bubble ${name === value ? 'border-2 border-fg' : 'border border-line'}`}
        >
          <Creature role="" face={name} size={36} />
        </button>
      ))}
    </div>
  );
}
