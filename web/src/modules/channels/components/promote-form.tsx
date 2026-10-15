import { useState } from 'react';
import { usePromoteChannel } from '@/modules/channels/api/promote-channel';
import type { Channel, Feature } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

export function PromoteForm({
  channel,
  features,
  disabled,
}: {
  channel: Channel;
  features: Feature[];
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [existing, setExisting] = useState(false);
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const [featureId, setFeatureId] = useState('');
  const promote = usePromoteChannel(channel.id);
  const choices = features.filter((f) => f.projectId === channel.projectId);

  if (!open) {
    return (
      <Button type="button" disabled={disabled} onClick={() => setOpen(true)}>
        Promote
      </Button>
    );
  }

  return (
    <form
      className="space-y-2 rounded border border-line p-3"
      onSubmit={(e) => {
        e.preventDefault();
        promote.mutate(
          existing ? { featureId: Number(featureId) } : { slug: slug.trim(), name: name.trim() },
          { onSuccess: () => setOpen(false) },
        );
      }}
    >
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={existing} onChange={(e) => setExisting(e.target.checked)} />
        Use an existing feature
      </label>
      {existing ? (
        <Select
          className="w-full"
          aria-label="Feature"
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
      ) : (
        <>
          <TextField
            className="w-full"
            aria-label="Feature slug"
            placeholder="slug"
            required
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
          />
          <TextField
            className="w-full"
            aria-label="Feature name"
            placeholder="name"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </>
      )}
      {promote.isError && <p className="text-sm text-danger">{promote.error.message}</p>}
      <div className="flex gap-2">
        <Button variant="primary" type="submit" loading={promote.isPending}>
          Promote
        </Button>
        <Button variant="ghost" type="button" onClick={() => setOpen(false)}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
