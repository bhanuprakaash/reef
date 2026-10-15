import { useState, type ReactNode } from 'react';
import { usePromoteChannel } from '@/modules/channels/api/promote-channel';
import type { Channel, Feature } from '@/types/api';
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
  channel,
  features,
  onClose,
}: {
  channel: Channel;
  features: Feature[];
  onClose: () => void;
}) {
  const [existing, setExisting] = useState(false);
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const [featureId, setFeatureId] = useState('');
  const promote = usePromoteChannel(channel.id);
  const choices = features.filter((f) => f.projectId === channel.projectId);

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        promote.mutate(
          existing ? { featureId: Number(featureId) } : { slug: slug.trim(), name: name.trim() },
          { onSuccess: onClose },
        );
      }}
    >
      <p className="-mt-2.5 text-[13px] text-muted">{channel.name} becomes long-lived work</p>
      <div role="radiogroup" aria-label="Where" className="grid grid-cols-2 gap-2">
        <Choice
          checked={!existing}
          title="New feature"
          note="Starts at Ideation"
          onChange={() => setExisting(false)}
        />
        <Choice
          checked={existing}
          title="Existing feature"
          note="Attach this channel"
          onChange={() => setExisting(true)}
        />
      </div>
      {existing ? (
        <Field label="Feature">
          <Select
            className="w-full"
            required
            value={featureId}
            onChange={(e) => setFeatureId(e.target.value)}
          >
            <option value="">Pick a feature</option>
            {choices.map((f) => (
              <option key={f.id} value={f.id}>
                {f.name}
              </option>
            ))}
          </Select>
        </Field>
      ) : (
        <div className="grid grid-cols-2 gap-2">
          <Field label="Name">
            <TextField
              className="w-full"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label="Slug">
            <TextField
              className="w-full font-mono"
              required
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
            />
          </Field>
        </div>
      )}
      <p className="text-[13px] text-muted">
        The channel moves under the feature and the lead reads it to{' '}
        {existing ? 'update the' : 'write the first'} brief (one turn).
        {!existing && slug.trim() !== '' && (
          <>
            {' '}
            Branch: <span className="font-mono text-soft">feature/{slug.trim()}</span>
          </>
        )}
      </p>
      {promote.isError && <p className="text-sm text-danger">{promote.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" loading={promote.isPending}>
          Promote
        </Button>
      </div>
    </form>
  );
}
