import type { ReactNode } from 'react';

export function Toast({ children }: { children: ReactNode }) {
  return (
    <div
      role="status"
      className="flex items-center gap-2.5 rounded-menu bg-pill px-3.5 py-2.5 text-[13.5px] text-pill-ink"
    >
      {children}
    </div>
  );
}
