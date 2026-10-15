import type { ComponentProps } from 'react';

type Variant = 'primary' | 'secondary' | 'ghost' | 'destructive';
type Size = 'sm' | 'md' | 'lg';

const variants: Record<Variant, string> = {
  primary: 'rounded-pill bg-pill px-4 text-pill-ink',
  secondary: 'rounded-pill border border-line px-4 text-fg hover:bg-card',
  ghost: 'rounded-field px-3 text-soft hover:bg-card',
  destructive: 'rounded-pill bg-danger-bg px-4 text-danger',
};

const sizes: Record<Size, string> = {
  sm: 'h-7 text-xs',
  md: 'h-9 text-sm',
  lg: 'h-11 text-[15px]',
};

export function buttonClass(variant: Variant = 'secondary', size: Size = 'md') {
  return `inline-flex items-center justify-center gap-2 disabled:cursor-not-allowed disabled:border-0 disabled:bg-track disabled:text-faint ${variants[variant]} ${sizes[size]}`;
}

type Props = ComponentProps<'button'> & { variant?: Variant; size?: Size; loading?: boolean };

export function Button({ variant, size, loading, className, disabled, children, ...rest }: Props) {
  return (
    <button
      className={`${buttonClass(variant, size)} ${className ?? ''}`}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {loading && (
        <svg
          className="size-3.5 animate-spin motion-reduce:animate-none"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="3"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <path d="M12 3a9 9 0 1 1-9 9" />
        </svg>
      )}
      {children}
    </button>
  );
}
