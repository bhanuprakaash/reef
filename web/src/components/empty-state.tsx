import type { ReactNode } from 'react';

type Props = { title: string; body?: string; children?: ReactNode };

export function EmptyState({ title, body, children }: Props) {
  return (
    <div className="flex flex-col items-center gap-2.5 rounded-menu bg-main p-7 text-center">
      <div className="text-[15px]">{title}</div>
      {body && <div className="text-[13px] text-muted">{body}</div>}
      {children}
    </div>
  );
}
