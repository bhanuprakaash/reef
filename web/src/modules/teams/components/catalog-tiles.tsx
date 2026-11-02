import { useState } from 'react';
import { useAddCatalogConnection } from '@/modules/teams/api/connections';
import {
  defaultUrl,
  entryFields,
  searchCatalog,
  type CatalogEntry,
} from '@/modules/teams/utils/catalog';
import { ConnectionLogo } from '@/modules/teams/components/connection-logo';
import type { Connection } from '@/types/api';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { TextField } from '@/components/text-field';

function CatalogTile({ entry, added }: { entry: CatalogEntry; added: boolean }) {
  const fields = entryFields(entry);
  const [open, setOpen] = useState(false);
  const [values, setValues] = useState(entry.defaults ?? {});
  const save = useAddCatalogConnection();
  const ready = fields.every((f) => f.optional || (values[f.key] ?? '').trim() !== '');

  function add() {
    setOpen(false);
    save.mutate({ entry, values });
  }

  return (
    <li className="flex flex-col gap-2 rounded-field border border-line bg-card p-3">
      <div className="flex items-center gap-2.5">
        <ConnectionLogo name={entry.name} entryId={entry.id} />
        <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{entry.name}</span>
      </div>
      <p className="line-clamp-2 text-xs text-muted">{entry.description}</p>
      <div className="mt-auto">
        {added && !save.isPending ? (
          <span className="text-xs text-muted">Added</span>
        ) : (
          <Button
            size="sm"
            aria-label={`Add ${entry.name}`}
            loading={save.isPending}
            onClick={fields.length > 0 ? () => setOpen(true) : add}
          >
            Add
          </Button>
        )}
      </div>
      {save.isError && <p className="text-xs text-danger">{save.error.message}</p>}
      {fields.length > 0 && (
        <Dialog open={open} onClose={() => setOpen(false)} title={`Add ${entry.name}`}>
          <form
            aria-label={`Add ${entry.name}`}
            className="flex flex-col gap-3.5"
            onSubmit={(e) => {
              e.preventDefault();
              add();
            }}
          >
            {fields.map((f) => (
              <div key={f.key} className="space-y-1.5">
                <label htmlFor={`${entry.id}-${f.key}`} className="block text-[12.5px] text-soft">
                  {f.label}
                  {f.optional && ' (optional)'}
                </label>
                <TextField
                  id={`${entry.id}-${f.key}`}
                  className="w-full font-mono text-[12.5px]"
                  type={f.secret ? 'password' : 'text'}
                  value={values[f.key] ?? ''}
                  placeholder={f.placeholder}
                  onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
                />
              </div>
            ))}
            <div className="flex justify-end gap-2 border-t border-hairline pt-3">
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" variant="primary" disabled={!ready}>
                Add
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </li>
  );
}

export function CatalogTiles({ connections }: { connections: Connection[] }) {
  const [query, setQuery] = useState('');
  const entries = searchCatalog(query);
  return (
    <div className="flex flex-col gap-3">
      <TextField
        type="search"
        aria-label="Search services"
        placeholder="Search services…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {entries.length === 0 && <p className="text-sm text-muted">No service matches.</p>}
      <ul className="grid grid-cols-2 gap-2.5 md:grid-cols-3">
        {entries.map((e) => (
          <CatalogTile
            key={e.id}
            entry={e}
            added={connections.some((c) => c.url === defaultUrl(e))}
          />
        ))}
      </ul>
    </div>
  );
}
