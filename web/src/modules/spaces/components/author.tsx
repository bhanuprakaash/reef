import { Creature, type Look } from '@/components/creature';

type Props = { name: string; look?: Look; time?: string };

export function Author({ name, look, time }: Props) {
  return (
    <div className="flex items-center gap-2 text-[13px] text-muted">
      {name !== 'user' && name !== 'reef' && <Creature look={look} size={20} />}
      <span className="capitalize">{name}</span>
      {time && <span>· {time}</span>}
    </div>
  );
}
