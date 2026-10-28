import { Markdown } from '@/components/markdown';

type Props = { own?: boolean; ringed?: boolean; className?: string; children: string };

export function Bubble({ own, ringed, className, children }: Props) {
  return (
    <div
      className={
        own
          ? `peer max-w-[75%] self-end rounded-[22px] bg-pill px-[18px] [--mention:var(--color-mention-on-pill)] py-2.5 text-pill-ink ${ringed ? 'ring-2 ring-fg ring-offset-2 ring-offset-main' : ''} ${className ?? ''}`
          : `peer max-w-[90%] rounded-[22px] bg-bubble px-[18px] py-3 ${ringed ? 'ring-2 ring-fg' : ''} ${className ?? ''}`
      }
    >
      <Markdown>{children}</Markdown>
    </div>
  );
}
