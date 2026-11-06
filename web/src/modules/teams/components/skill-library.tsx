import { useState } from 'react';
import { useBots } from '@/lib/api/get-bots';
import {
  useImportSkill,
  useSkillDrafts,
  useSkillFiles,
  useSkillLibrary,
  useUpdateSkill,
} from '@/modules/teams/api/skill-library';
import { SkillDraftCard } from '@/modules/teams/components/skill-draft-card';
import { SkillFileList } from '@/modules/teams/components/skill-file-list';
import { EditSkillEditor, NewSkillEditor } from '@/modules/teams/components/skill-editor';
import type { LibrarySkill, SkillInfo } from '@/types/api';
import { Button } from '@/components/button';
import { SegmentedControl } from '@/components/segmented-control';
import { TextField } from '@/components/text-field';

function describeSource(s: LibrarySkill) {
  if (s.sourceKind === 'reef') return 'Written in Reef';
  if (s.sourceKind === 'folder') return `Folder ${s.source}`;
  const where = [s.source, s.ref && `ref ${s.ref}`, s.dir && `in ${s.dir}`].filter(Boolean);
  return `Git ${where.join(' ')} at ${s.revision.slice(0, 12)}`;
}

function SkillFiles({ skill }: { skill: LibrarySkill }) {
  const files = useSkillFiles(skill.id, skill.version, true);
  if (files.isError) return <p className="text-sm text-danger">{files.error.message}</p>;
  return <SkillFileList files={files.data?.files ?? []} />;
}

type Group = 'library' | 'claude' | 'codex';

type Entry = {
  key: string;
  ref: string;
  name: string;
  description: string;
  group: Group;
  library?: LibrarySkill;
};

const groups: { value: 'all' | Group; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'library', label: 'Library' },
  { value: 'claude', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
];

const tagClass = 'rounded-pill bg-bubble px-2.5 py-0.5 text-[11.5px] text-soft';

function SkillDetail({ entry, usedBy }: { entry: Entry; usedBy: string[] }) {
  const update = useUpdateSkill();
  const [editing, setEditing] = useState(false);
  const skill = entry.library;
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2.5">
        <h3 className="font-mono text-[17px] font-normal">{entry.name}</h3>
        {skill && <span className={tagClass}>v{skill.version}</span>}
        <span className={tagClass}>{groups.find((g) => g.value === entry.group)!.label}</span>
        {skill &&
          (skill.sourceKind === 'reef' ? (
            <Button
              className="ml-auto"
              size="sm"
              aria-label={editing ? undefined : `Edit ${skill.name}`}
              onClick={() => setEditing(!editing)}
            >
              {editing ? 'Cancel' : 'Edit'}
            </Button>
          ) : (
            <Button
              className="ml-auto"
              size="sm"
              aria-label={`Update ${skill.name}`}
              loading={update.isPending}
              onClick={() => update.mutate(skill.id)}
            >
              Update
            </Button>
          ))}
      </div>
      {entry.description && <p className="mt-2 text-soft">{entry.description}</p>}
      <p className="break-all text-[13px] text-muted">
        {skill ? describeSource(skill) : 'Read from disk. Read-only.'}
      </p>
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      {skill && editing ? (
        <div className="mt-4">
          <EditSkillEditor skill={skill} onDone={() => setEditing(false)} />
        </div>
      ) : (
        <>
          {skill && (
            <>
              <h4 className="mt-4 text-[11px] uppercase tracking-[0.08em] text-muted">Files</h4>
              <SkillFiles skill={skill} />
            </>
          )}
          <h4 className="mt-4 text-[11px] uppercase tracking-[0.08em] text-muted">Used by</h4>
          {usedBy.length === 0 ? (
            <p className="text-[13px] text-muted">No Bot has it.</p>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              {usedBy.map((name) => (
                <span key={name} className={tagClass}>
                  {name}
                </span>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function ImportForm({ onDone }: { onDone: () => void }) {
  const [kind, setKind] = useState<'folder' | 'git'>('folder');
  const [path, setPath] = useState('');
  const [repo, setRepo] = useState('');
  const [ref, setRef] = useState('');
  const [dir, setDir] = useState('');
  const save = useImportSkill();
  const ready = kind === 'folder' ? path.trim() !== '' : repo.trim() !== '';

  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(
          kind === 'folder'
            ? { path: path.trim() }
            : { repo: repo.trim(), ref: ref.trim(), dir: dir.trim() },
          {
            onSuccess: onDone,
          },
        );
      }}
    >
      <SegmentedControl
        label="Import from"
        options={[
          { value: 'folder', label: 'Folder' },
          { value: 'git', label: 'Git link' },
        ]}
        value={kind}
        onChange={setKind}
      />
      {kind === 'folder' ? (
        <TextField
          aria-label="Skill folder path"
          placeholder="/path/to/skill-folder"
          value={path}
          onChange={(e) => setPath(e.target.value)}
        />
      ) : (
        <>
          <TextField
            aria-label="Git repository link"
            placeholder="https://github.com/owner/repo"
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
          />
          <div className="flex flex-wrap gap-2">
            <TextField
              aria-label="Branch or tag"
              placeholder="Branch or tag (optional)"
              className="min-w-[160px] flex-1"
              value={ref}
              onChange={(e) => setRef(e.target.value)}
            />
            <TextField
              aria-label="Folder in the repository"
              placeholder="Folder in the repo (optional)"
              className="min-w-[160px] flex-1"
              value={dir}
              onChange={(e) => setDir(e.target.value)}
            />
          </div>
        </>
      )}
      <div className="flex items-center gap-3">
        <Button type="submit" variant="primary" disabled={!ready} loading={save.isPending}>
          Import
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
        {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
      </div>
    </form>
  );
}

export function SkillLibrary({ disk }: { disk: SkillInfo[] }) {
  const library = useSkillLibrary();
  const drafts = useSkillDrafts();
  const bots = useBots();
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<'all' | Group>('all');
  const [picked, setPicked] = useState('');

  const entries: Entry[] = [
    ...(library.data ?? []).map((s) => ({
      key: `library:${s.id}`,
      ref: `reef:${s.name}`,
      name: s.name,
      description: s.description,
      group: 'library' as const,
      library: s,
    })),
    ...disk
      .filter((s) => s.source !== 'reef')
      .map((s) => ({
        key: s.ref,
        ref: s.ref,
        name: s.name,
        description: s.description,
        group: s.source as Group,
      })),
  ];
  const q = query.trim().toLowerCase();
  const shown = entries.filter(
    (e) =>
      (filter === 'all' || e.group === filter) &&
      (e.name.toLowerCase().includes(q) || e.description.toLowerCase().includes(q)),
  );
  const current = shown.find((e) => e.key === picked) ?? shown[0];

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <TextField
          type="search"
          aria-label="Filter skills"
          placeholder="Search skills"
          className="min-w-[200px] flex-1"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <SegmentedControl
          label="Source"
          options={groups.map((g) => ({
            value: g.value,
            label: (
              <>
                {g.label}{' '}
                <span className="text-muted">
                  {entries.filter((e) => g.value === 'all' || e.group === g.value).length}
                </span>
              </>
            ),
          }))}
          value={filter}
          onChange={setFilter}
        />
        <Button size="sm" onClick={() => setImporting(!importing)}>
          Import
        </Button>
        <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
          New skill
        </Button>
      </div>
      {importing && (
        <div className="rounded-card border border-hairline bg-card p-4">
          <ImportForm onDone={() => setImporting(false)} />
        </div>
      )}
      {library.isError && <p className="text-sm text-danger">{library.error.message}</p>}
      <div className="flex flex-col overflow-hidden rounded-card border border-hairline bg-sidebar md:h-[468px] md:flex-row">
        <ul
          aria-label="Skill list"
          className="max-h-[280px] shrink-0 overflow-y-auto border-b border-hairline md:max-h-none md:w-[290px] md:border-r md:border-b-0"
        >
          {shown.length === 0 && (
            <li className="p-5 text-sm text-muted">
              {entries.length === 0 ? 'No skills yet.' : 'No skills match.'}
            </li>
          )}
          {shown.map((e) => (
            <li key={e.key} className="border-b border-hairline last:border-b-0">
              <button
                type="button"
                aria-current={!creating && e.key === current?.key ? 'true' : undefined}
                onClick={() => {
                  setPicked(e.key);
                  setCreating(false);
                }}
                className={`flex min-h-11 w-full flex-col gap-px px-3.5 py-2.5 text-left hover:bg-hover/50 ${!creating && e.key === current?.key ? 'bg-active' : ''}`}
              >
                <span className="flex items-center gap-2 font-mono text-[13px]">
                  {e.name}
                  <span className="ml-auto font-sans text-[11px] text-faint">
                    {e.library
                      ? `v${e.library.version}`
                      : groups.find((g) => g.value === e.group)!.label}
                  </span>
                </span>
                <span className="truncate text-xs text-muted">{e.description}</span>
              </button>
            </li>
          ))}
        </ul>
        <div className="min-w-0 flex-1 overflow-y-auto bg-main p-5">
          {creating ? (
            <NewSkillEditor
              onDone={(saved) => {
                setCreating(false);
                if (saved) setPicked(`library:${saved.id}`);
              }}
            />
          ) : current ? (
            <SkillDetail
              key={current.key}
              entry={current}
              usedBy={(bots.data ?? [])
                .filter((b) => b.skills.includes(current.ref))
                .map((b) => b.name)}
            />
          ) : (
            <p className="pt-12 text-center text-muted">
              Pick a skill to see its files and where it is used.
            </p>
          )}
        </div>
      </div>
      {drafts.data && drafts.data.length > 0 && (
        <div className="flex flex-col gap-2">
          <h3 className="text-[13px]">Drafts</h3>
          {drafts.data.map((d) => (
            <SkillDraftCard key={d.id} draft={d} />
          ))}
        </div>
      )}
    </div>
  );
}
