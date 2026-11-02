import { useCallback, useRef, useState, type ReactNode } from 'react';
import { Button } from '@/components/button';
import { Menu, MenuItem } from '@/components/menu';
import { useDismiss } from '@/lib/use-dismiss';

type Item = { label: string; onClick: () => void; destructive?: boolean };

const MENU_WIDTH = 160;
const MENU_HEIGHT = 140;

export function RowMenu({ items, children }: { items: Item[]; children: ReactNode }) {
  const [at, setAt] = useState<{ x: number; y: number } | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setAt(null), []);

  useDismiss(at !== null, ref, close);

  return (
    <div
      ref={ref}
      className="group relative"
      onContextMenu={(e) => {
        e.preventDefault();
        setAt({ x: e.clientX, y: e.clientY });
      }}
    >
      {children}
      <Button
        type="button"
        variant="ghost"
        size="iconSm"
        aria-label="More actions"
        aria-haspopup="menu"
        aria-expanded={at !== null}
        title="More actions"
        onClick={(e) => {
          const box = e.currentTarget.getBoundingClientRect();
          setAt(at ? null : { x: box.left, y: box.bottom });
        }}
        className="absolute right-2 bottom-1.5 opacity-0 group-hover:opacity-100 hover:text-fg focus-visible:opacity-100 focus-visible:ring-3 focus-visible:ring-ring aria-expanded:opacity-100 max-md:right-0 max-md:bottom-1/2 max-md:size-11 max-md:translate-y-1/2 max-md:opacity-100 pointer-coarse:opacity-100"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <circle cx="5" cy="12" r="1.8" />
          <circle cx="12" cy="12" r="1.8" />
          <circle cx="19" cy="12" r="1.8" />
        </svg>
      </Button>
      {at && (
        <Menu
          className="fixed z-30"
          style={{
            width: MENU_WIDTH,
            left: Math.min(at.x, window.innerWidth - MENU_WIDTH - 8),
            top: Math.min(at.y, window.innerHeight - MENU_HEIGHT),
          }}
        >
          {items.map((item) => (
            <MenuItem
              key={item.label}
              destructive={item.destructive}
              onClick={() => {
                close();
                item.onClick();
              }}
            >
              {item.label}
            </MenuItem>
          ))}
        </Menu>
      )}
    </div>
  );
}
