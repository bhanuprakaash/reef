import { useProviders } from '@/lib/api/get-providers';
import { useRemoveProvider } from '@/modules/providers/api/remove-provider';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { claudeModels } from '@/utils/models';
import type { Provider } from '@/types/api';

function models(kind: string) {
  return kind === 'claude' ? claudeModels.map((m) => m.label).join(', ') : 'Any, set on each role';
}

export function ProviderRow({
  provider,
  onRemove,
  removing,
}: {
  provider: Provider;
  onRemove: () => void;
  removing?: boolean;
}) {
  const details: [string, string | undefined][] = [
    ['Path', provider.path ?? 'Binary not found'],
    ['Version', provider.version ?? 'Unknown'],
    ['Folder', provider.folder],
    ['Models', models(provider.kind)],
  ];

  return (
    <li className="flex flex-col gap-2 py-3">
      <div className="flex min-h-9 items-center gap-2">
        <span className="min-w-0 truncate font-mono text-[13px]">{provider.name}</span>
        <Chip>{provider.kind}</Chip>
        <Chip>{provider.detected ? 'Found on PATH' : 'Added'}</Chip>
        {!provider.detected && (
          <Button
            size="sm"
            className="ml-auto"
            aria-label={`Remove ${provider.name}`}
            loading={removing}
            onClick={onRemove}
          >
            Remove
          </Button>
        )}
      </div>
      <dl className="grid grid-cols-[64px_minmax(0,1fr)] gap-x-3 gap-y-1 text-[12.5px]">
        {details.map(
          ([term, value]) =>
            value && (
              <div key={term} className="contents">
                <dt className="text-muted">{term}</dt>
                <dd title={value} className="truncate">
                  {value}
                </dd>
              </div>
            ),
        )}
      </dl>
    </li>
  );
}

export function ProviderList() {
  const providers = useProviders();
  const remove = useRemoveProvider();

  return (
    <>
      <p className="text-[13px] text-muted">
        The claude and codex installs Reef can run. Add one from another folder, then pick it on a
        Bot.
      </p>
      {providers.isPending && <p className="text-sm text-muted">Loading...</p>}
      {providers.isError && <p className="text-sm text-danger">{providers.error.message}</p>}
      {providers.data?.length === 0 && (
        <p className="text-sm text-muted">No claude or codex found on PATH.</p>
      )}
      <ul className="flex flex-col divide-y divide-hairline">
        {providers.data?.map((p) => (
          <ProviderRow
            key={p.name}
            provider={p}
            removing={remove.isPending && remove.variables === p.name}
            onRemove={() => remove.mutate(p.name)}
          />
        ))}
      </ul>
      {remove.isError && <p className="text-sm text-danger">{remove.error.message}</p>}
    </>
  );
}
