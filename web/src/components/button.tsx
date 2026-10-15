import type { ComponentProps } from 'react';

type Variant = 'primary' | 'secondary' | 'ghost' | 'destructive' | 'paused' | 'failed';
type Size = 'sm' | 'md' | 'lg' | 'icon';

const variants: Record<Variant, string> = {
  primary: 'bg-pill text-pill-ink',
  secondary: 'border border-line text-fg hover:bg-card',
  ghost: 'text-soft hover:bg-card',
  destructive: 'bg-danger-bg text-danger',
  paused: 'bg-paused text-paused-ink',
  failed: 'border border-failed-line text-failed-fg',
};

const sizes: Record<Size, string> = {
  sm: 'h-11 text-xs md:h-7',
  md: 'h-11 text-sm md:h-9',
  lg: 'h-11 text-[15px]',
  icon: 'size-11 md:size-10',
};

export function buttonClass(variant: Variant = 'secondary', size: Size = 'md') {
  const shape =
    size === 'icon'
      ? 'rounded-full'
      : variant === 'ghost'
        ? 'rounded-field px-3'
        : 'rounded-pill px-4';
  return `inline-flex items-center justify-center gap-2 disabled:cursor-not-allowed disabled:border-0 disabled:bg-track disabled:text-faint ${shape} ${variants[variant]} ${sizes[size]}`;
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
