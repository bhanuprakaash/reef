import { useState } from 'react';
import { useProviders } from '@/lib/api/get-providers';
import { AddProviderDialog } from '@/modules/providers/components/add-provider-dialog';
import { useRemoveProvider } from '@/modules/providers/api/remove-provider';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { ProviderLogo } from '@/components/provider-logo';
import { StatusDot } from '@/components/status-dot';
import { claudeModels } from '@/utils/models';
import type { Provider } from '@/types/api';

export function ProviderRow({
  provider,
  onRemove,
  removing,
}: {
  provider: Provider;
  onRemove: () => void;
  removing?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const ready = Boolean(provider.path);
  const details: [string, string | undefined][] = [
    ['Source', provider.detected ? 'Found on PATH' : 'Added from a folder'],
    ['Folder', provider.folder],
  ];

  return (
    <li className="border-t border-hairline first:border-t-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-3 px-3.5 py-3 text-left hover:bg-active"
      >
        <span className="flex size-[30px] flex-none items-center justify-center rounded-field border border-line bg-card">
          <ProviderLogo kind={provider.kind} className="size-[17px]" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2 text-[13px] font-medium">
            <span className="truncate">{provider.name}</span>
            {provider.name !== provider.kind && (
              <span className="rounded-md border border-line px-1.5 text-[11px] leading-[17px] font-normal text-muted">
                {provider.kind}
              </span>
            )}
          </span>
          <span className="block truncate text-xs text-muted">
            {provider.version && `${provider.version} · `}
            {provider.detected ? 'PATH' : provider.folder}
          </span>
        </span>
        <span className="flex items-center gap-1.5 text-xs whitespace-nowrap text-soft">
          <StatusDot status={ready ? 'done' : 'paused'} />
          {ready ? 'Ready' : 'Binary not found'}
        </span>
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className={`flex-none text-faint transition-transform ${open ? 'rotate-90' : ''}`}
        >
          <path d="m9 6 6 6-6 6" />
        </svg>
      </button>
      <div hidden={!open} className="px-3.5 pb-3.5 md:pl-14">
        <dl className="mb-3 grid grid-cols-[72px_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-[12.5px]">
          {details.map(
            ([term, value]) =>
              value && (
                <div key={term} className="contents">
                  <dt className="text-muted">{term}</dt>
                  <dd title={value} className={`truncate ${term === 'Folder' ? 'font-mono' : ''}`}>
                    {value}
                  </dd>
                </div>
              ),
          )}
          <dt className="text-muted">Binary</dt>
          <dd
            title={provider.path}
            className={`truncate ${provider.path ? 'font-mono' : 'text-danger'}`}
          >
            {provider.path ?? 'Not found'}
          </dd>
          <dt className="text-muted">Version</dt>
          <dd>{provider.version ?? 'Unknown'}</dd>
          <dt className="text-muted">Models</dt>
          <dd className="flex flex-wrap gap-1.5">
            {provider.kind === 'claude' ? (
              claudeModels.map((m) => <Chip key={m.value}>{m.label}</Chip>)
            ) : (
              <span className="text-faint">Any model, set on each Bot</span>
            )}
          </dd>
        </dl>
        {provider.detected ? (
          <p className="text-xs text-faint">
            Found on PATH. Reef always lists it; it cannot be removed.
          </p>
        ) : (
          <div className="flex justify-end">
            <Button
              size="sm"
              variant="destructive"
              aria-label={`Remove ${provider.name}`}
              loading={removing}
              onClick={onRemove}
            >
              Remove
            </Button>
          </div>
        )}
      </div>
    </li>
  );
}

export function ProviderList() {
  const providers = useProviders();
  const remove = useRemoveProvider();
  const [adding, setAdding] = useState(false);

  return (
    <>
      {providers.isPending && <p className="text-sm text-muted">Loading...</p>}
      {providers.isError && <p className="text-sm text-danger">{providers.error.message}</p>}
      {providers.data?.length === 0 && (
        <p className="text-sm text-muted">No claude or codex found on PATH.</p>
      )}
      <div className="overflow-hidden rounded-card border border-hairline bg-sidebar">
        <div className="flex items-center gap-2 border-b border-hairline px-3.5 py-2.5">
          <h3 className="flex-1 text-[13px] font-medium">Installs</h3>
          <Button size="sm" onClick={() => setAdding(true)}>
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M12 5v14M5 12h14" />
            </svg>
            Add provider
          </Button>
        </div>
        <ul>
          {providers.data?.map((p) => (
            <ProviderRow
              key={p.name}
              provider={p}
              removing={remove.isPending && remove.variables === p.name}
              onRemove={() => remove.mutate(p.name)}
            />
          ))}
        </ul>
      </div>
      {remove.isError && <p className="text-sm text-danger">{remove.error.message}</p>}
      <AddProviderDialog open={adding} onClose={() => setAdding(false)} />
    </>
  );
}
