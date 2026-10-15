import type { ComponentProps } from 'react';

export function Menu({ className, ...rest }: ComponentProps<'div'>) {
  return (
    <div
      role="menu"
      className={`rounded-menu border border-line bg-card p-1.5 shadow-menu ${className ?? ''}`}
      {...rest}
    />
  );
}

export function MenuItem({
  destructive,
  className,
  ...rest
}: ComponentProps<'button'> & { destructive?: boolean }) {
  return (
    <button
      type="button"
      role="menuitem"
      className={`flex h-[34px] w-full items-center gap-2.5 rounded-lg px-2.5 text-left text-sm hover:bg-hover ${destructive ? 'text-danger' : 'text-fg'} ${className ?? ''}`}
      {...rest}
    />
  );
}

export function MenuSeparator() {
  return <div role="separator" className="mx-1 my-1.5 h-px bg-line" />;
}
