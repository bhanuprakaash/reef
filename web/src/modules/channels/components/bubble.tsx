import type { ReactNode } from 'react';

type Props = { own?: boolean; ringed?: boolean; children: ReactNode };

export function Bubble({ own, ringed, children }: Props) {
  return (
    <div
      className={`whitespace-pre-wrap rounded-[22px] px-[18px] ${
        own ? 'max-w-[72%] self-end bg-pill py-2.5 text-pill-ink' : 'max-w-[80%] bg-bubble py-3'
      } ${ringed ? `ring-2 ring-fg ${own ? 'ring-offset-2 ring-offset-main' : ''}` : ''}`}
    >
      {children}
    </div>
  );
}
