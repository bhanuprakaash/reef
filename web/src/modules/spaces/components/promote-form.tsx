import { useState, type ReactNode } from 'react';
import { usePromoteSpace } from '@/modules/spaces/api/promote-space';
import type { Space, WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

function Choice({
  checked,
  title,
  note,
  onChange,
}: {
  checked: boolean;
  title: string;
  note: string;
  onChange: () => void;
}) {
  return (
    <label
      className={`flex flex-col gap-0.5 rounded-menu border p-3 ${checked ? 'border-fg bg-track' : 'border-line'}`}
    >
      <span className="flex items-center gap-2">
        <input
          type="radio"
          name="promote-where"
          checked={checked}
          onChange={onChange}
          className="accent-fg"
        />
        {title}
      </span>
      <span className="pl-[22px] text-[12.5px] text-muted">{note}</span>
    </label>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12.5px] text-muted">{label}</span>
      {children}
    </label>
  );
}

export function PromoteForm({
  space,
  features,
  onClose,
}: {
  space: Space;
  features: WorkItem[];
  onClose: () => void;
}) {
  const [existing, setExisting] = useState(false);
  const [title, setTitle] = useState('');
  const [featureId, setFeatureId] = useState('');
  const promote = usePromoteSpace(space.id);
  const choices = features.filter((f) => f.projectId === space.projectId);

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        promote.mutate(existing ? { workItemId: Number(featureId) } : { title: title.trim() }, {
          onSuccess: onClose,
        });
      }}
    >
      <p className="-mt-2.5 text-[13px] text-muted">{space.name} becomes long-lived work</p>
      <div role="radiogroup" aria-label="Where" className="grid grid-cols-2 gap-2">
        <Choice
          checked={!existing}
          title="New feature"
          note="Starts in the backlog"
          onChange={() => setExisting(false)}
        />
        <Choice
          checked={existing}
          title="Existing feature"
          note="Attach this Space"
          onChange={() => setExisting(true)}
        />
      </div>
      {existing ? (
        <Field label="Feature">
          <Select
            className="w-full"
            aria-label="Feature"
            value={featureId}
            onChange={setFeatureId}
            options={[
              { value: '', label: 'Pick a feature', disabled: true },
              ...choices.map((f) => ({ value: String(f.id), label: f.title })),
            ]}
          />
        </Field>
      ) : (
        <Field label="Title">
          <TextField
            className="w-full"
            required
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
        </Field>
      )}
      <p className="text-[13px] text-muted">
        The Space moves under the feature and the lead reads it to{' '}
        {existing ? 'update the' : 'write the first'} brief (one turn).
      </p>
      {promote.isError && <p className="text-sm text-danger">{promote.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="primary"
          type="submit"
          disabled={existing && featureId === ''}
          loading={promote.isPending}
        >
          Promote
        </Button>
      </div>
    </form>
  );
}
