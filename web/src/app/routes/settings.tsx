import { useState, type ReactNode } from 'react';
import { Button } from '@/components/button';
import { getTheme, setTheme, type Theme } from '@/lib/theme';
import { useNotificationPermission } from '@/lib/use-notification-permission';

// The previews depict each theme, so they keep literal colours.
const themes: { value: Theme; label: string; preview: ReactNode }[] = [
  {
    value: 'dark',
    label: 'Dark',
    preview: (
      <span className="flex h-full bg-[#212121]">
        <span className="w-[30%] rounded-l-[10px] bg-[#1a1a1a]" />
      </span>
    ),
  },
  {
    value: 'light',
    label: 'Light',
    preview: (
      <span className="flex h-full bg-white">
        <span className="w-[30%] rounded-l-[10px] bg-[#f3f3f4]" />
      </span>
    ),
  },
  {
    value: 'system',
    label: 'Match macOS',
    preview: <span className="block h-full bg-[linear-gradient(90deg,#212121_50%,#ffffff_50%)]" />,
  },
];

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section
      aria-label={title}
      className="flex flex-col gap-3 rounded-card bg-sidebar px-[18px] py-4"
    >
      <h2 className="text-[13px] font-normal text-muted">{title}</h2>
      {children}
    </section>
  );
}

export function SettingsPage() {
  const [theme, setChosen] = useState(getTheme);
  const { permission, request } = useNotificationPermission();

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-[22px] py-6">
      <h1 className="text-[32px] font-normal tracking-[-0.02em]">Settings</h1>

      <Section title="Appearance">
        <div role="radiogroup" aria-label="Theme" className="grid grid-cols-3 gap-2.5">
          {themes.map((t) => (
            <label key={t.value} className="flex flex-col gap-2 text-[13px]">
              <span
                className={`block h-[70px] overflow-hidden rounded-xl border-2 ${t.value === theme ? 'border-fg' : 'border-line'}`}
              >
                {t.preview}
              </span>
              <span className="flex items-center gap-1.5">
                <input
                  type="radio"
                  name="theme"
                  checked={t.value === theme}
                  onChange={() => {
                    setTheme(t.value);
                    setChosen(t.value);
                  }}
                  className="accent-fg"
                />
                {t.label}
              </span>
            </label>
          ))}
        </div>
      </Section>

      <Section title="Notifications">
        <div className="flex min-h-9 items-center gap-3">
          <span className="flex-1">Mac notification when something needs you</span>
          {permission === 'default' && (
            <Button type="button" size="sm" onClick={request}>
              Turn on notifications
            </Button>
          )}
          {permission === 'granted' && <span className="text-[13px] text-done">On</span>}
          {permission === 'denied' && (
            <span className="text-[13px] text-muted">Blocked in system settings</span>
          )}
        </div>
      </Section>
    </div>
  );
}
