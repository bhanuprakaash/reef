import type { ReactNode } from 'react';

function Toggle({
  label,
  pressed,
  onClick,
  children,
}: {
  label: string;
  pressed: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      onClick={onClick}
      className="flex size-8 items-center justify-center rounded-lg text-soft hover:bg-hover/50 aria-pressed:bg-active aria-pressed:text-fg"
    >
      <svg
        width="17"
        height="17"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.7"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {children}
      </svg>
    </button>
  );
}

export function PaneToggles({
  browserOpen,
  onBrowser,
  terminalOpen,
  onTerminal,
}: {
  browserOpen: boolean;
  onBrowser: () => void;
  terminalOpen: boolean;
  onTerminal?: () => void;
}) {
  return (
    <div className="absolute top-3 right-3 z-30 flex gap-0.5">
      <Toggle label="Browser" pressed={browserOpen} onClick={onBrowser}>
        <circle cx="12" cy="12" r="9" />
        <path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18" />
      </Toggle>
      {onTerminal && (
        <Toggle label="Terminal" pressed={terminalOpen} onClick={onTerminal}>
          <rect x="3" y="4" width="18" height="16" rx="3" />
          <path d="M7 9l3 3-3 3M13 15h4" />
        </Toggle>
      )}
    </div>
  );
}
