import type { ReactNode } from 'react';

export function Bubble({ own, children }: { own?: boolean; children: ReactNode }) {
  return (
    <div
      className={`whitespace-pre-wrap rounded-[22px] px-[18px] ${
        own ? 'max-w-[72%] self-end bg-pill py-2.5 text-pill-ink' : 'max-w-[80%] bg-bubble py-3'
      }`}
    >
      {children}
    </div>
  );
}
