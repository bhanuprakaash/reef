import type { ComponentProps, ReactNode } from 'react';
import { Link } from 'react-router';

export function Row(props: {
  to: string;
  active: boolean;
  avatar: ReactNode;
  title: string;
  live?: ReactNode;
  aside?: ReactNode;
  trigger?: ComponentProps<'a'>;
}) {
  return (
    <Link
      {...props.trigger}
      to={props.to}
      className={`flex h-[34px] items-center gap-2.5 rounded-xl px-2.5 max-md:h-11 max-md:pr-12 ${props.active ? 'bg-active' : 'hover:bg-hover/50'}`}
    >
      <span className="flex w-[26px] flex-none justify-center">{props.avatar}</span>
      <span className="truncate text-sm">{props.title}</span>
      {props.live}
      <span className="ml-auto flex shrink-0 items-center gap-1.5 text-xs text-muted">
        {props.aside}
      </span>
    </Link>
  );
}
