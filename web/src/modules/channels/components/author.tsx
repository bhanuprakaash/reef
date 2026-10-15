import { Creature } from '@/components/creature';

type Props = { name: string; face?: string; time?: string };

export function Author({ name, face, time }: Props) {
  return (
    <div className="flex items-center gap-2 text-[13px] text-muted">
      {name !== 'user' && name !== 'reef' && <Creature role={name} face={face} size={20} />}
      <span className="capitalize">{name}</span>
      {time && <span>· {time}</span>}
    </div>
  );
}
