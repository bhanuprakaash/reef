import type { ReactNode } from 'react';

type Props = {
  title: string;
  open?: boolean;
  onToggle?: (open: boolean) => void;
  children: ReactNode;
};

export function Fold({ title, open, onToggle, children }: Props) {
  return (
    <details
      open={open}
      onToggle={onToggle && ((e) => onToggle(e.currentTarget.open))}
      className="group border-t border-hairline"
    >
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 text-sm font-medium [&::-webkit-details-marker]:hidden">
        {title}
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="text-faint transition-transform group-open:rotate-90"
        >
          <path d="m9 6 6 6-6 6" />
        </svg>
      </summary>
      <div className="pb-2">{children}</div>
    </details>
  );
}
