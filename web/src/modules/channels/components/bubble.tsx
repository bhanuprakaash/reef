import { Markdown } from '@/components/markdown';

type Props = { own?: boolean; ringed?: boolean; children: string };

export function Bubble({ own, ringed, children }: Props) {
  return (
    <div
      className={
        own
          ? `max-w-[75%] self-end rounded-[22px] bg-pill px-[18px] py-2.5 text-pill-ink ${ringed ? 'ring-2 ring-fg ring-offset-2 ring-offset-main' : ''}`
          : `w-full ${ringed ? '-ml-3 border-l-2 border-fg pl-2.5' : ''}`
      }
    >
      <Markdown>{children}</Markdown>
    </div>
  );
}
