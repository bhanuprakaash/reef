import { useState } from 'react';
import {
  useImportSkill,
  useSkillFiles,
  useSkillLibrary,
  useUpdateSkill,
} from '@/modules/teams/api/skill-library';
import type { LibrarySkill } from '@/types/api';
import { Button } from '@/components/button';
import { SegmentedControl } from '@/components/segmented-control';
import { TextField } from '@/components/text-field';

function describeSource(s: LibrarySkill) {
  if (s.sourceKind === 'folder') return `Folder ${s.source}`;
  const where = [s.source, s.ref && `ref ${s.ref}`, s.dir && `in ${s.dir}`].filter(Boolean);
  return `Git ${where.join(' ')} at ${s.revision.slice(0, 12)}`;
}

function SkillFiles({ skill }: { skill: LibrarySkill }) {
  const files = useSkillFiles(skill.id, skill.version, true);
  if (files.isError) return <p className="text-sm text-danger">{files.error.message}</p>;
  return (
    <ul className="mt-2 flex flex-col gap-1">
      {files.data?.files.map((f) => (
        <li key={f.path}>
          <details>
            <summary className="cursor-pointer font-mono text-xs text-soft">{f.path}</summary>
            <pre className="mt-1 max-h-60 overflow-auto rounded-field bg-field p-2 text-xs whitespace-pre-wrap">
              {f.body}
            </pre>
          </details>
        </li>
      ))}
    </ul>
  );
}

function SkillRow({ skill }: { skill: LibrarySkill }) {
  const update = useUpdateSkill();
  const [viewing, setViewing] = useState(false);
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
        <Button
          size="sm"
          aria-label={`Update ${skill.name}`}
          loading={update.isPending}
          onClick={() => update.mutate(skill.id)}
        >
          Update
        </Button>
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
  return (
    <section aria-label="Skills library" className="flex flex-col gap-3">
      <div>
        <h2 className="text-[15px]">Skills library</h2>
        <p className="mt-1 text-[13px] text-muted">
          Import a skill folder or a Git link. Importing never runs its scripts. An update is
          explicit and saves a new version; the old ones stay.
        </p>
      </div>
      <ImportForm />
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
