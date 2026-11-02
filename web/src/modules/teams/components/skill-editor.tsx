import { useState } from 'react';
import { useCreateSkill, useEditSkill, useSkillFiles } from '@/modules/teams/api/skill-library';
import type { LibrarySkill, SkillFile } from '@/types/api';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

const MANIFEST = 'SKILL.md';
const TEMPLATE = '---\nname: \ndescription: \n---\n\n';

function EditorForm({
  skill,
  initial,
  onDone,
}: {
  skill?: LibrarySkill;
  initial: SkillFile[];
  onDone: (saved?: LibrarySkill) => void;
}) {
  const [name, setName] = useState(skill?.name ?? '');
  const [manifest, setManifest] = useState(
    initial.find((f) => f.path === MANIFEST)?.body ?? TEMPLATE,
  );
  const [others, setOthers] = useState(initial.filter((f) => f.path !== MANIFEST));
  const create = useCreateSkill();
  const edit = useEditSkill(skill?.id ?? 0);
  const save = skill ? edit : create;
  const files = [{ path: MANIFEST, body: manifest }, ...others];
  const change = (i: number, patch: Partial<SkillFile>) =>
    setOthers(others.map((f, j) => (j === i ? { ...f, ...patch } : f)));

  return (
    <form
      aria-label={skill ? `Edit ${skill.name}` : 'New skill'}
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (skill) edit.mutate(files, { onSuccess: onDone });
        else create.mutate({ name: name.trim(), files }, { onSuccess: onDone });
      }}
    >
      <TextField
        aria-label="Skill name"
        placeholder="skill-name"
        value={name}
        disabled={skill !== undefined}
        onChange={(e) => setName(e.target.value)}
      />
      <TextArea
        aria-label="SKILL.md"
        rows={10}
        className="font-mono text-xs"
        value={manifest}
        onChange={(e) => setManifest(e.target.value)}
      />
      {others.map((f, i) => (
        <div key={i} className="flex flex-col gap-1">
          <div className="flex gap-2">
            <TextField
              aria-label="Supporting file path"
              placeholder="notes/how.md"
              className="flex-1"
              value={f.path}
              onChange={(e) => change(i, { path: e.target.value })}
            />
            <Button
              type="button"
              size="sm"
              aria-label={`Remove ${f.path || 'file'}`}
              onClick={() => setOthers(others.filter((_, j) => j !== i))}
            >
              Remove
            </Button>
          </div>
          <TextArea
            aria-label={`Contents of ${f.path || 'file'}`}
            rows={5}
            className="font-mono text-xs"
            value={f.body}
            onChange={(e) => change(i, { body: e.target.value })}
          />
        </div>
      ))}
      <div className="flex flex-wrap items-center gap-3">
        <Button
          type="button"
          size="sm"
          onClick={() => setOthers([...others, { path: '', body: '' }])}
        >
          Add file
        </Button>
        <Button
          type="submit"
          variant="primary"
          disabled={name.trim() === ''}
          loading={save.isPending}
        >
          {skill ? 'Save new version' : 'Create skill'}
        </Button>
        <Button type="button" variant="ghost" onClick={() => onDone()}>
          Cancel
        </Button>
        {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
      </div>
    </form>
  );
}

export function NewSkillEditor({ onDone }: { onDone: (saved?: LibrarySkill) => void }) {
  return <EditorForm initial={[]} onDone={onDone} />;
}

export function EditSkillEditor({ skill, onDone }: { skill: LibrarySkill; onDone: () => void }) {
  const files = useSkillFiles(skill.id, skill.version, true);
  if (files.isError) return <p className="text-sm text-danger">{files.error.message}</p>;
  if (!files.data) return <p className="text-sm text-muted">Loading...</p>;
  return <EditorForm skill={skill} initial={files.data.files} onDone={onDone} />;
}
