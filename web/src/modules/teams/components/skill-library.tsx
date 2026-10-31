import { useState } from 'react';
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
import type { LibrarySkill } from '@/types/api';
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

function SkillRow({ skill }: { skill: LibrarySkill }) {
  const update = useUpdateSkill();
  const [viewing, setViewing] = useState(false);
  const [editing, setEditing] = useState(false);
  if (editing) {
    return (
      <li className="py-3">
        <EditSkillEditor skill={skill} onDone={() => setEditing(false)} />
      </li>
    );
  }
  return (
    <li className="py-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-[13px]">{skill.name}</span>
        <span className="text-xs text-muted">v{skill.version}</span>
        <Button
          className="ml-auto"
          size="sm"
          aria-label={`View files of ${skill.name}`}
          aria-pressed={viewing}
          onClick={() => setViewing(!viewing)}
        >
          Files
        </Button>
        {skill.sourceKind === 'reef' ? (
          <Button size="sm" aria-label={`Edit ${skill.name}`} onClick={() => setEditing(true)}>
            Edit
          </Button>
        ) : (
          <Button
            size="sm"
            aria-label={`Update ${skill.name}`}
            loading={update.isPending}
            onClick={() => update.mutate(skill.id)}
          >
            Update
          </Button>
        )}
      </div>
      {skill.description && <p className="mt-0.5 text-[12.5px] text-muted">{skill.description}</p>}
      <p className="mt-0.5 break-all text-xs text-muted">{describeSource(skill)}</p>
      {update.isError && <p className="mt-1 text-sm text-danger">{update.error.message}</p>}
      {viewing && <SkillFiles skill={skill} />}
    </li>
  );
}

function ImportForm() {
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
            onSuccess: () => {
              setPath('');
              setRepo('');
              setRef('');
              setDir('');
            },
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
        {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
      </div>
    </form>
  );
}

export function SkillLibrary() {
  const library = useSkillLibrary();
  const drafts = useSkillDrafts();
  const [creating, setCreating] = useState(false);
  return (
    <section aria-label="Skills library" className="flex flex-col gap-3">
      <div>
        <h2 className="text-[15px]">Skills library</h2>
        <p className="mt-1 text-[13px] text-muted">
          Import a skill folder or a Git link, or write one here. Importing never runs its scripts.
          An update or an edit is explicit and saves a new version; the old ones stay. Neither gives
          the skill to a Bot.
        </p>
      </div>
      <ImportForm />
      {creating ? (
        <NewSkillEditor onDone={() => setCreating(false)} />
      ) : (
        <div>
          <Button size="sm" onClick={() => setCreating(true)}>
            New skill
          </Button>
        </div>
      )}
      {drafts.data && drafts.data.length > 0 && (
        <div className="flex flex-col gap-2">
          <h3 className="text-[13px]">Drafts</h3>
          {drafts.data.map((d) => (
            <SkillDraftCard key={d.id} draft={d} />
          ))}
        </div>
      )}
      {library.isError && <p className="text-sm text-danger">{library.error.message}</p>}
      {library.data?.length === 0 && <p className="text-sm text-muted">No skills imported yet.</p>}
      <ul className="flex flex-col divide-y divide-hairline">
        {library.data?.map((s) => (
          <SkillRow key={s.id} skill={s} />
        ))}
      </ul>
    </section>
  );
}
