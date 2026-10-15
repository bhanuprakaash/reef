import { Creature } from '@/components/creature';

export function Author({ name }: { name: string }) {
  return (
    <div className="flex items-center gap-2 text-[13px] capitalize text-muted">
      {name !== 'user' && name !== 'reef' && <Creature role={name} size={20} />}
      {name}
    </div>
  );
}
