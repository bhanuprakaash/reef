import { useState } from 'react';
import { useAddProvider } from '@/modules/providers/api/add-provider';
import { Button } from '@/components/button';
import { SegmentedControl } from '@/components/segmented-control';
import { TextField } from '@/components/text-field';

export function AddProviderForm() {
  const [kind, setKind] = useState<'claude' | 'codex'>('claude');
  const [name, setName] = useState('');
  const [folder, setFolder] = useState('');
  const add = useAddProvider();

  return (
    <form
      aria-label="Add provider"
      className="flex flex-col gap-3 border-t border-hairline pt-4"
      onSubmit={(e) => {
        e.preventDefault();
        add.mutate(
          { name, kind, folder },
          {
            onSuccess: () => {
              setName('');
              setFolder('');
            },
          },
        );
      }}
    >
      <h3 className="text-[13px] text-muted">Add a provider</h3>
      <SegmentedControl
        label="Kind"
        options={[
          { value: 'claude', label: 'claude' },
          { value: 'codex', label: 'codex' },
        ]}
        value={kind}
        onChange={setKind}
        className="self-start"
      />
      <div className="space-y-1.5">
        <label htmlFor="provider-folder" className="block text-[12.5px] text-muted">
          Folder
        </label>
        <div className="flex gap-2">
          <TextField
            id="provider-folder"
            className="min-w-0 flex-1 font-mono"
            placeholder={`/Users/you/${kind}-work`}
            value={folder}
            onChange={(e) => setFolder(e.target.value)}
            required
          />
          {window.reef && (
            <Button
              type="button"
              onClick={() => void window.reef?.chooseFolder().then((p) => p && setFolder(p))}
            >
              Choose…
            </Button>
          )}
        </div>
      </div>
      <div className="space-y-1.5">
        <label htmlFor="provider-name" className="block text-[12.5px] text-muted">
          Name
        </label>
        <TextField
          id="provider-name"
          className="w-full"
          placeholder={`work-${kind}`}
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </div>
      {add.isError && <p className="text-sm text-danger">{add.error.message}</p>}
      <div className="flex justify-end">
        <Button variant="primary" type="submit" loading={add.isPending}>
          Add provider
        </Button>
      </div>
    </form>
  );
}
