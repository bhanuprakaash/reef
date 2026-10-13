import { useState } from 'react';
import { usePromoteChannel } from '@/modules/channels/api/promote-channel';
import type { Channel, Feature } from '@/types/api';
import { buttonClass, errorClass, inputClass } from '@/utils/classes';

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
      <button
        type="button"
        className={buttonClass}
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        Promote
      </button>
    );
  }

  return (
    <form
      className="space-y-2 rounded border border-neutral-300 p-3 dark:border-neutral-700"
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
        <select
          className={`${inputClass} w-full`}
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
        </select>
      ) : (
        <>
          <input
            className={`${inputClass} w-full`}
            aria-label="Feature slug"
            placeholder="slug"
            required
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
          />
          <input
            className={`${inputClass} w-full`}
            aria-label="Feature name"
            placeholder="name"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </>
      )}
      {promote.isError && <p className={errorClass}>{promote.error.message}</p>}
      <div className="flex gap-2">
        <button type="submit" className={buttonClass} disabled={promote.isPending}>
          Promote
        </button>
        <button type="button" className={buttonClass} onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
}
