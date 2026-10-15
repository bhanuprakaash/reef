import type { ComponentProps } from 'react';
import { fieldClass } from '@/components/text-field';

export function TextArea({ className, ...rest }: ComponentProps<'textarea'>) {
  return <textarea className={`${fieldClass} py-2.5 ${className ?? ''}`} {...rest} />;
}
