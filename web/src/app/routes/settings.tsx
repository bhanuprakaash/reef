import { Fragment, useEffect, useState, type ReactNode } from 'react';
import { useSearchParams } from 'react-router';
import { ProfileFields } from '@/app/profile-fields';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';
import { useProfile } from '@/lib/api/get-profile';
import { useProviders } from '@/lib/api/get-providers';
import { useSkills } from '@/lib/api/get-skills';
import { useCurrentProject } from '@/lib/current-project';
import { setTerminalApp, useTerminalApp } from '@/lib/terminal-app';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useConnections } from '@/modules/teams/api/connections';
import { ConnectionLibrary } from '@/modules/teams/components/connection-library';
import { SkillLibrary } from '@/modules/teams/components/skill-library';
import { RepoConnectionList } from '@/modules/repo-connections/components/repo-connection-list';
import { ProviderList } from '@/modules/providers/components/provider-list';
import { usePutProfile } from '@/lib/api/put-profile';
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

function Row({ children }: { children: ReactNode }) {
  return <div className="flex min-h-[54px] items-center gap-4 py-2.5">{children}</div>;
}

function NameSection() {
  const profile = useProfile();
  const save = usePutProfile();
  const [name, setName] = useState<string | null>(null);
  const saved = {
    name: profile.data?.name ?? '',
    occupation: profile.data?.occupation ?? '',
    style: profile.data?.style ?? [],
  };
  const shown = save.isPending ? save.variables : saved;
  const put = (next: Partial<typeof saved>) => save.mutate({ ...shown, ...next });

  return (
    <>
      <div className="flex flex-col divide-y divide-hairline">
        <Row>
          <label htmlFor="profile-name" className="flex-1 text-[13px] text-muted">
            Your name
          </label>
          <TextField
            id="profile-name"
            className="min-w-0 flex-1 md:w-[280px] md:flex-none"
            value={name ?? saved.name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
            onBlur={() => {
              const next = (name ?? saved.name).trim();
              setName(null);
              if (next !== '' && next !== saved.name) put({ name: next });
            }}
          />
        </Row>
        <div className="flex flex-col gap-3 py-3">
          <ProfileFields
            occupation={shown.occupation}
            style={shown.style}
            onOccupation={(occupation) => put({ occupation })}
            onStyle={(style) => put({ style })}
          />
        </div>
        <Row>
          <span className="flex-1 text-[13px] text-muted">Changes save as you make them.</span>
          {save.isSuccess && <span className="text-[13px] text-done">Saved</span>}
        </Row>
      </div>
      {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
    </>
  );
}

type UpdateState =
  | { status: 'idle' | 'checking' | 'current' }
  | { status: 'ready'; version: string }
  | { status: 'error'; message: string };

function UpdateSection() {
  const [idle, setIdle] = useState(false);
  const [current, setCurrent] = useState<string | null>(null);
  const [update, setUpdate] = useState<UpdateState>({ status: 'idle' });
  useEffect(() => {
    void window.reef?.getUpdateWhenIdle().then(setIdle);
    void window.reef?.getVersion().then(setCurrent);
    return window.reef?.onUpdateReady((version) => setUpdate({ status: 'ready', version }));
  }, []);

  return (
    <>
      <div className="flex min-h-9 items-center gap-3">
        <span className="flex-1">Version {current ?? ''}</span>
        {update.status === 'ready' ? (
          <Button variant="primary" size="sm" onClick={() => window.reef?.restartToUpdate()}>
            Restart to update
          </Button>
        ) : (
          <Button
            size="sm"
            loading={update.status === 'checking'}
            onClick={() => {
              setUpdate({ status: 'checking' });
              void window.reef?.checkForUpdate().then(setUpdate);
            }}
          >
            Check for updates
          </Button>
        )}
      </div>
      <p
        role="status"
        className={`text-[13px] ${update.status === 'error' ? 'text-danger' : 'text-muted'}`}
      >
        {update.status === 'checking' && 'Checking…'}
        {update.status === 'current' && 'Reef is up to date.'}
        {update.status === 'ready' && `Reef ${update.version} is ready. Restart to install it.`}
        {update.status === 'error' && update.message}
      </p>
      <div className="flex min-h-9 items-center gap-3">
        <span className="flex-1">Update when idle</span>
        <Toggle
          label="Update when idle"
          checked={idle}
          onChange={(value) => {
            setIdle(value);
            void window.reef?.setUpdateWhenIdle(value);
          }}
        />
      </div>
      <p className="text-[13px] text-muted">
        Restart into a downloaded update once no agent turn is running.
      </p>
    </>
  );
}

function AppearanceSection() {
  const [theme, setChosen] = useState(getTheme);

  return (
    <>
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
    </>
  );
}

function NotificationsSection() {
  const { permission, request } = useNotificationPermission();

  return (
    <>
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
    </>
  );
}

function TerminalSection() {
  const app = useTerminalApp();
  const [installed, setInstalled] = useState<string[] | null>(null);
  const [custom, setCustom] = useState(false);
  useEffect(() => {
    void window.reef?.terminalApps().then(setInstalled);
  }, []);
  const isCustom = installed !== null && (custom || (app !== null && !installed.includes(app)));
  const choices = [
    { label: 'Built-in pane', checked: !isCustom && app === null, app: null },
    ...(installed ?? []).map((name) => ({
      label: name,
      checked: !isCustom && app === name,
      app: name,
    })),
  ];

  return (
    <>
      {installed && (
        <div role="radiogroup" aria-label="Terminal app" className="flex flex-col gap-2">
          {choices.map((c) => (
            <label key={c.label} className="flex items-center gap-1.5 text-[13px]">
              <input
                type="radio"
                name="terminal-app"
                checked={c.checked}
                onChange={() => {
                  setCustom(false);
                  setTerminalApp(c.app);
                }}
                className="accent-fg"
              />
              {c.label}
            </label>
          ))}
          <label className="flex items-center gap-1.5 text-[13px]">
            <input
              type="radio"
              name="terminal-app"
              checked={isCustom}
              onChange={() => {
                setCustom(true);
                setTerminalApp(null);
              }}
              className="accent-fg"
            />
            Custom
          </label>
          {isCustom && (
            <TextField
              aria-label="Custom terminal app"
              placeholder="App name, e.g. Kitty"
              value={app ?? ''}
              onChange={(e) => setTerminalApp(e.target.value || null)}
            />
          )}
        </div>
      )}
    </>
  );
}

const sections: {
  id: string;
  title: string;
  group: string;
  lede: string;
  desktopOnly?: boolean;
}[] = [
  { id: 'profile', title: 'Profile', group: 'You', lede: 'Reef tells your Bots who you are.' },
  { id: 'appearance', title: 'Appearance', group: 'You', lede: 'Dark, light, or match macOS.' },
  {
    id: 'notifications',
    title: 'Notifications',
    group: 'You',
    lede: 'Reef can tell you when a Work item needs you.',
  },
  {
    id: 'terminal',
    title: 'Terminal',
    group: 'This Mac',
    lede: 'Where the Terminal toggle opens a Work item’s worktree.',
    desktopOnly: true,
  },
  {
    id: 'updates',
    title: 'Updates',
    group: 'This Mac',
    lede: 'Check for a new Reef, then restart to install it.',
    desktopOnly: true,
  },
  {
    id: 'providers',
    title: 'Providers',
    group: 'Agents',
    lede: 'The claude and codex installs Reef can run. Add one from another folder, then pick it on a Bot.',
  },
  {
    id: 'connections',
    title: 'Connections',
    group: 'Agents',
    lede: 'Tools a Bot can use, such as a filesystem, GitLab or a database. Add one here, then give it to a Bot from its panel.',
  },
  {
    id: 'skills',
    title: 'Skills',
    group: 'Agents',
    lede: 'Reef’s library, plus the Claude Code and Codex skills Reef finds on disk. Importing never runs scripts. Neither gives a skill to a Bot.',
  },
  {
    id: 'repositories',
    title: 'Repositories',
    group: 'Work',
    lede: 'GitHub and GitLab, through the gh and glab logins on this Mac. Log in with the CLI, then connect here; Reef keeps the account name, never the token.',
  },
];

export function SettingsPage() {
  const [params, setParams] = useSearchParams();
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const skills = useSkills(project?.id);
  const providers = useProviders();
  const connections = useConnections();
  const counts: Record<string, number | undefined> = {
    providers: providers.data?.length,
    connections: connections.data?.length,
    skills: skills.data?.length,
  };
  const panes: Record<string, ReactNode> = {
    profile: <NameSection />,
    appearance: <AppearanceSection />,
    notifications: <NotificationsSection />,
    terminal: <TerminalSection />,
    updates: <UpdateSection />,
    providers: <ProviderList />,
    repositories: <RepoConnectionList />,
    connections: <ConnectionLibrary projectId={project?.id} />,
    skills: <SkillLibrary disk={skills.data ?? []} />,
  };
  const shown = sections.filter((s) => !s.desktopOnly || window.reef);
  const open = shown.find((s) => s.id === params.get('section'));
  const current = open ?? shown[0];

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-[22px] py-6">
      <h1 className="text-[32px] font-normal tracking-[-0.02em]">Settings</h1>
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:gap-9">
        <nav
          aria-label="Settings sections"
          className={`shrink-0 flex-col gap-px md:flex md:w-[176px] ${open ? 'hidden' : 'flex'}`}
        >
          {shown.map((s, i) => (
            <Fragment key={s.id}>
              {s.group !== shown[i - 1]?.group && (
                <div className="px-3 pt-3.5 pb-1.5 text-[11px] uppercase tracking-[0.08em] text-faint first:pt-0.5">
                  {s.group}
                </div>
              )}
              <button
                type="button"
                aria-current={s.id === current.id ? 'page' : undefined}
                onClick={() => setParams({ section: s.id }, { replace: true })}
                className={`flex h-11 items-center rounded-[10px] px-3 text-left text-[14px] md:h-[34px] ${s.id === current.id ? `text-fg ${open ? 'bg-active' : 'md:bg-active'}` : 'text-muted hover:bg-hover/50'}`}
              >
                {s.title}
                {counts[s.id] !== undefined && (
                  <span className="ml-auto text-[11.5px] text-faint">{counts[s.id]}</span>
                )}
              </button>
            </Fragment>
          ))}
        </nav>
        <section
          aria-label={current.title}
          className={`min-w-0 flex-1 md:block ${open ? 'block' : 'hidden'}`}
        >
          <button
            type="button"
            className="mb-3 flex h-11 items-center text-[13px] text-muted md:hidden"
            onClick={() => setParams({}, { replace: true })}
          >
            ‹ Settings
          </button>
          <h2 className="text-[20px] font-normal tracking-[-0.01em]">{current.title}</h2>
          <p className="mt-[3px] text-[13px] text-muted">{current.lede}</p>
          <div className="mt-3.5 flex flex-col gap-3">{panes[current.id]}</div>
        </section>
      </div>
    </div>
  );
}
