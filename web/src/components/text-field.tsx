import type { ComponentProps } from 'react';

export const fieldClass =
  'rounded-field border border-line bg-field px-3 text-sm text-fg placeholder:text-muted outline-none focus-visible:border-fg focus-visible:ring-3 focus-visible:ring-ring aria-invalid:border-failed disabled:cursor-not-allowed disabled:text-faint';

export function TextField({ className, ...rest }: ComponentProps<'input'>) {
  return <input className={`${fieldClass} h-[38px] ${className ?? ''}`} {...rest} />;
}
