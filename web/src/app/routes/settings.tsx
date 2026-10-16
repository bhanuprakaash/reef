import { useEffect, useState, type ReactNode } from 'react';
import { ProfileFields } from '@/app/profile-fields';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';
import { useProfile } from '@/lib/api/get-profile';
import { useSkills } from '@/lib/api/get-skills';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { usePutProfile } from '@/lib/api/put-profile';
import type { SkillInfo } from '@/types/api';
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

function NameSection() {
  const profile = useProfile();
  const save = usePutProfile();
  const [draft, setDraft] = useState<{ name: string; occupation: string; style: string[] } | null>(
    null,
  );
  const saved = {
    name: profile.data?.name ?? '',
    occupation: profile.data?.occupation ?? '',
    style: profile.data?.style ?? [],
  };
  const { name, occupation, style } = draft ?? saved;
  const changed =
    name.trim() !== saved.name ||
    occupation !== saved.occupation ||
    style.join('\n') !== saved.style.join('\n');

  return (
    <Section title="Profile">
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ name, occupation, style }, { onSuccess: () => setDraft(null) });
        }}
      >
        <div className="flex items-center gap-3">
          <label htmlFor="profile-name" className="text-[13px] text-muted">
            Your name
          </label>
          <TextField
            id="profile-name"
            className="min-w-0 flex-1"
            value={name}
            onChange={(e) => setDraft({ name: e.target.value, occupation, style })}
          />
        </div>
        <ProfileFields
          occupation={occupation}
          style={style}
          onOccupation={(o) => setDraft({ name, occupation: o, style })}
          onStyle={(s) => setDraft({ name, occupation, style: s })}
        />
        <div className="flex justify-end">
          <Button
            variant="primary"
            size="md"
            type="submit"
            loading={save.isPending}
            disabled={name.trim() === '' || !changed}
          >
            Save
          </Button>
        </div>
      </form>
      {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
    </Section>
  );
}

function UpdateSection() {
  const [idle, setIdle] = useState(false);
  useEffect(() => {
    void window.reef?.getUpdateWhenIdle().then(setIdle);
  }, []);

  return (
    <Section title="Updates">
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
    </Section>
  );
}

function SkillGroup({ title, skills }: { title: string; skills: SkillInfo[] }) {
  return (
    <div aria-label={title} role="group" className="flex flex-col gap-y-1.5">
      <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">{title}</h3>
      {skills.length === 0 && <p className="text-[13px] text-muted">None found.</p>}
      <ul className="flex flex-col gap-y-1.5">
        {skills.map((s) => (
          <li key={s.name} className="text-[13px]">
            <span className="font-mono">{s.name}</span>
            {s.description && <span className="text-muted"> {s.description}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

function SkillsSection() {
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const skills = useSkills(project?.id);

  return (
    <Section title="Skills">
      <p className="text-[13px] text-muted">
        Read from disk for {project ? project.name : 'the current project'}. Roles pick from the
        list of their backend.
      </p>
      {!project && <p className="text-sm text-muted">Add a project to see its skills.</p>}
      {skills.isError && <p className="text-sm text-danger">{skills.error.message}</p>}
      {skills.data && (
        <>
          <SkillGroup title="Claude Code" skills={skills.data.claude} />
          <SkillGroup title="Codex" skills={skills.data.codex} />
        </>
      )}
    </Section>
  );
}

export function SettingsPage() {
  const [theme, setChosen] = useState(getTheme);
  const { permission, request } = useNotificationPermission();

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-[22px] py-6">
      <h1 className="text-[32px] font-normal tracking-[-0.02em]">Settings</h1>

      <NameSection />

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

      {window.reef && <UpdateSection />}

      <SkillsSection />

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
