import { Markdown } from '@/components/markdown';

type Props = { own?: boolean; ringed?: boolean; children: string };

export function Bubble({ own, ringed, children }: Props) {
  return (
    <div
      className={
        own
          ? `max-w-[75%] self-end rounded-[22px] bg-pill px-[18px] [--mention:var(--color-mention-on-pill)] py-2.5 text-pill-ink ${ringed ? 'ring-2 ring-fg ring-offset-2 ring-offset-main' : ''}`
          : `max-w-[90%] rounded-[22px] bg-bubble px-[18px] py-3 ${ringed ? 'ring-2 ring-fg' : ''}`
      }
    >
      <Markdown>{children}</Markdown>
    </div>
  );
}
