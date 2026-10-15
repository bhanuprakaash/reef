import type { ComponentProps } from 'react';

export function Chip({ className, ...rest }: ComponentProps<'span'>) {
  return (
    <span
      className={`inline-flex h-6 items-center gap-1.5 rounded-pill bg-bubble px-2.5 text-xs text-soft ${className ?? ''}`}
      {...rest}
    />
  );
}
