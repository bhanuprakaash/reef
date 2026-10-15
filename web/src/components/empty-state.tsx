import type { ReactNode } from 'react';
import { Creature } from '@/components/creature';

type Props = {
  title: string;
  body?: string;
  role?: string;
  compact?: boolean;
  children?: ReactNode;
};

export function EmptyState({ title, body, role, compact, children }: Props) {
  const creature = role && <Creature role={role} state="asleep" size={compact ? 30 : 56} animate />;
  if (compact) {
    return (
      <div className="flex items-center gap-3">
        {creature}
        <div className="min-w-0">
          <div className="text-[13px] text-soft">{title}</div>
          {body && <div className="text-[12.5px] text-muted">{body}</div>}
        </div>
        {children}
      </div>
    );
  }
  return (
    <div className="flex flex-col items-center gap-2.5 rounded-menu bg-main p-7 text-center">
      {creature}
      <div className="text-[15px]">{title}</div>
      {body && <div className="text-[13px] text-muted">{body}</div>}
      {children}
    </div>
  );
}
