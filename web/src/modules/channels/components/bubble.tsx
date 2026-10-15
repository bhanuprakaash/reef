import { Markdown } from '@/components/markdown';

type Props = { own?: boolean; ringed?: boolean; children: string };

export function Bubble({ own, ringed, children }: Props) {
  return (
    <div
      className={`rounded-[22px] px-[18px] ${
        own ? 'max-w-[72%] self-end bg-pill py-2.5 text-pill-ink' : 'max-w-[80%] bg-bubble py-3'
      } ${ringed ? `ring-2 ring-fg ${own ? 'ring-offset-2 ring-offset-main' : ''}` : ''}`}
    >
      <Markdown>{children}</Markdown>
    </div>
  );
}
