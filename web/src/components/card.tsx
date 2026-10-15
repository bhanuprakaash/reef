import { useId, type ReactNode } from 'react';

type Props = { title: string; aside?: ReactNode; flush?: boolean; children: ReactNode };

export function Card({ title, aside, flush, children }: Props) {
  const id = useId();
  return (
    <section
      aria-labelledby={id}
      className="overflow-hidden rounded-card border border-hairline bg-card"
    >
      <div className="flex items-center gap-2.5 px-6 pt-[18px] pb-2.5">
        <h2 id={id} className="text-[15px] font-medium">
          {title}
        </h2>
        {aside}
      </div>
      <div className={flush ? '' : 'px-6 pb-5'}>{children}</div>
    </section>
  );
}
