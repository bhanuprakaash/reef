import type { ReactNode } from 'react';
import { Link } from 'react-router';

export function Row(props: {
  to: string;
  active: boolean;
  avatar: ReactNode;
  title: string;
  live?: ReactNode;
  aside?: ReactNode;
  preview: string;
}) {
  return (
    <Link
      to={props.to}
      className={`flex items-center gap-3 rounded-xl px-2.5 py-[9px] ${props.active ? 'bg-active' : 'hover:bg-hover/50'}`}
    >
      {props.avatar}
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-2">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate text-[14.5px]">{props.title}</span>
            {props.live}
          </span>
          <span className="ml-auto shrink-0 text-xs text-muted">{props.aside}</span>
        </span>
        <span className="block truncate text-[13px] text-muted">{props.preview}</span>
      </span>
    </Link>
  );
}
