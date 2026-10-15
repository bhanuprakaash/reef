import type { ComponentProps } from 'react';
import { fieldClass } from '@/components/text-field';

export function Select({ className, ...rest }: ComponentProps<'select'>) {
  return <select className={`${fieldClass} h-11 md:h-[38px] ${className ?? ''}`} {...rest} />;
}
