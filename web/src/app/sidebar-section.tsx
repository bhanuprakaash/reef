import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { Button } from '@/components/button';
import { useStoredFlag } from '@/lib/use-stored-flag';

const CAP = 8;

export function shownRows<T>(items: T[], isPinned: (item: T) => boolean) {
  const pinned = items.filter(isPinned);
  const rest = items.filter((item) => !isPinned(item));
  return [...pinned, ...rest.slice(0, Math.max(0, CAP - pinned.length))];
}

export function SidebarSection<T extends { id: number }>({
  id,
  to,
  label,
  addLabel,
  onAdd,
  empty,
  items,
  isPinned,
  row,
}: {
  id: string;
  to: string;
  label: string;
  addLabel: string;
  onAdd: () => void;
  empty: string;
  items: T[];
  isPinned: (item: T) => boolean;
  row: (item: T) => ReactNode;
}) {
  const [collapsed, toggle] = useStoredFlag(`reef.sidebar.collapsed.${id}`);
  return (
    <>
      <div className="flex items-center justify-between pt-2.5 pr-1">
        <h4 className="text-xs">
          <button
            type="button"
            aria-expanded={!collapsed}
            onClick={toggle}
            className="flex items-center gap-1 rounded-lg px-2 py-1.5 text-muted outline-none hover:bg-hover/50 hover:text-fg focus-visible:ring-3 focus-visible:ring-ring max-md:py-3"
          >
            <svg
              width="10"
              height="10"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.4"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={`transition-transform motion-reduce:transition-none ${collapsed ? '-rotate-90' : ''}`}
              aria-hidden="true"
            >
              <path d="M6 9l6 6 6-6" />
            </svg>
            {label}
            {collapsed && <span className="text-faint">{items.length}</span>}
          </button>
        </h4>
        <Button
          type="button"
          variant="ghost"
          size="iconSm"
          aria-label={addLabel}
          title={addLabel}
          onClick={onAdd}
          className="hover:text-fg max-md:size-11"
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M12 5v14M5 12h14" />
          </svg>
        </Button>
      </div>
      {!collapsed && (
        <>
          {items.length === 0 && <p className="px-2.5 pb-1 text-[13px] text-faint">{empty}</p>}
          <ul className="flex flex-col gap-0.5">
            {shownRows(items, isPinned).map((item) => (
              <li key={item.id}>{row(item)}</li>
            ))}
          </ul>
          {items.length > 0 && (
            <Link
              to={to}
              className="flex h-7 items-center gap-1 rounded-lg pl-[46px] text-[13px] text-muted outline-none hover:bg-hover/50 hover:text-fg focus-visible:ring-3 focus-visible:ring-ring max-md:h-11"
            >
              See all {items.length}
              <svg
                width="12"
                height="12"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M9 6l6 6-6 6" />
              </svg>
            </Link>
          )}
        </>
      )}
    </>
  );
}
