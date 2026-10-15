import { Creature } from '@/components/creature';

export function Author({ name }: { name: string }) {
  return (
    <div className="flex items-center gap-1.5 text-xs font-medium uppercase text-muted">
      {name !== 'user' && name !== 'reef' && <Creature role={name} size={20} />}
      {name}
    </div>
  );
}
