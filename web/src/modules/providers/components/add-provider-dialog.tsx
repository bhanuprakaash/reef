import { useState } from 'react';
import { useAddProvider } from '@/modules/providers/api/add-provider';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { ProviderLogo } from '@/components/provider-logo';
import { TextField } from '@/components/text-field';

const kinds = [
  { value: 'claude', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
];

export function AddProviderDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [step, setStep] = useState<1 | 2>(1);
  const [kind, setKind] = useState('');
  const [name, setName] = useState('');
  const [folder, setFolder] = useState('');
  const add = useAddProvider();

  function closed() {
    setStep(1);
    setKind('');
    setName('');
    setFolder('');
    add.reset();
    onClose();
  }

  return (
    <Dialog open={open} onClose={closed} title="Add provider">
      <div hidden={step !== 1}>
        <p className="mb-3 text-[13px] text-muted">Which CLI is in the folder?</p>
        <div className="mb-4 grid grid-cols-2 gap-2.5">
          {kinds.map((k) => (
            <button
              key={k.value}
              type="button"
              aria-pressed={kind === k.value}
              onClick={() => setKind(k.value)}
              className="flex items-center gap-2.5 rounded-field border border-line bg-card p-3 text-left hover:bg-hover aria-pressed:border-fg aria-pressed:ring-1 aria-pressed:ring-fg"
            >
              <span className="flex size-[30px] flex-none items-center justify-center rounded-field border border-line bg-sidebar">
                <ProviderLogo kind={k.value} className="size-[17px]" />
              </span>
              <span className="text-sm">
                {k.label}
                <small className="block text-xs text-muted">{k.value} binary</small>
              </span>
            </button>
          ))}
        </div>
        <div className="flex items-center justify-between gap-2 border-t border-hairline pt-3">
          <span className="text-xs text-faint">Step 1 of 2</span>
          <div className="flex gap-2">
            <Button variant="ghost" type="button" onClick={closed}>
              Cancel
            </Button>
            <Button variant="primary" type="button" disabled={!kind} onClick={() => setStep(2)}>
              Next
            </Button>
          </div>
        </div>
      </div>
      <form
        aria-label="Add provider"
        hidden={step !== 2}
        onSubmit={(e) => {
          e.preventDefault();
          add.mutate({ name, kind, folder }, { onSuccess: onClose });
        }}
      >
        <div className="flex flex-col gap-3.5">
          <div className="space-y-1.5">
            <label htmlFor="provider-folder" className="block text-[12.5px] text-soft">
              Folder with the binary
            </label>
            <div className="flex gap-2">
              <TextField
                id="provider-folder"
                className="min-w-0 flex-1 font-mono text-[12.5px]"
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
            <label htmlFor="provider-name" className="block text-[12.5px] text-soft">
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
            <p className="text-xs text-faint">Shown on the Bot’s provider picker.</p>
          </div>
        </div>
        {add.isError && <p className="mt-3 text-sm text-danger">{add.error.message}</p>}
        <div className="mt-4 flex items-center justify-between gap-2 border-t border-hairline pt-3">
          <span className="text-xs text-faint">Step 2 of 2</span>
          <div className="flex gap-2">
            <Button variant="ghost" type="button" onClick={() => setStep(1)}>
              Back
            </Button>
            <Button variant="primary" type="submit" loading={add.isPending}>
              Add provider
            </Button>
          </div>
        </div>
      </form>
    </Dialog>
  );
}
