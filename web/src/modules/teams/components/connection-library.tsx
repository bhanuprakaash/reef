import { useState } from 'react';
import { useBots } from '@/lib/api/get-bots';
import {
  useConnections,
  useDeleteConnection,
  useLoginConnection,
  useSaveConnection,
  type ConnectionInput,
} from '@/modules/teams/api/connections';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import { connectionLabel, pairsText, parsePairs } from '@/modules/teams/utils/connections';
import type { Connection } from '@/types/api';
import { AvailableConnections } from '@/modules/teams/components/available-connections';
import { Button } from '@/components/button';
import { SegmentedControl } from '@/components/segmented-control';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

function ConnectionForm({ initial, onDone }: { initial?: Connection; onDone: () => void }) {
  const [name, setName] = useState(initial?.name ?? '');
  const [kind, setKind] = useState<'command' | 'url'>(initial?.url ? 'url' : 'command');
  const [command, setCommand] = useState(initial?.command ?? '');
  const [args, setArgs] = useState((initial?.args ?? []).join('\n'));
  const [env, setEnv] = useState(pairsText(initial?.env));
  const [url, setUrl] = useState(initial?.url ?? '');
  const [headers, setHeaders] = useState(pairsText(initial?.headers, ': '));
  const save = useSaveConnection();
  const ready =
    name.trim() !== '' && (kind === 'command' ? command.trim() !== '' : url.trim() !== '');

  function submit() {
    const input: ConnectionInput =
      kind === 'command'
        ? {
            name: name.trim(),
            command: command.trim(),
            args: args.split('\n').filter((a) => a !== ''),
            env: parsePairs(env),
          }
        : {
            name: name.trim(),
            url: url.trim(),
            headers: parsePairs(headers, ': '),
            oauth: initial?.oauth,
          };
    save.mutate({ id: initial?.id, input }, { onSuccess: onDone });
  }

  return (
    <form
      className="flex flex-col gap-2 py-3"
      aria-label={initial ? `Edit ${initial.name}` : 'Add connection'}
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <TextField
        aria-label="Connection name"
        placeholder="Name, e.g. files"
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <SegmentedControl
        label="Connection type"
        options={[
          { value: 'command', label: 'Local process' },
          { value: 'url', label: 'HTTP' },
        ]}
        value={kind}
        onChange={setKind}
      />
      {kind === 'command' ? (
        <>
          <TextField
            className="font-mono"
            aria-label="Connection command"
            placeholder="Command, e.g. npx"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
          />
          <TextArea
            className="font-mono"
            rows={2}
            aria-label="Connection arguments"
            placeholder="Arguments, one per line"
            value={args}
            onChange={(e) => setArgs(e.target.value)}
          />
          <TextArea
            className="font-mono"
            rows={2}
            aria-label="Connection environment"
            placeholder="Environment, KEY=value one per line"
            value={env}
            onChange={(e) => setEnv(e.target.value)}
          />
          {initial && (
            <p className="text-xs text-muted">
              Saved secrets show as ****; leave them to keep the stored value.
            </p>
          )}
        </>
      ) : (
        <>
          <TextField
            className="font-mono"
            aria-label="Connection URL"
            placeholder="https://host/mcp"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <TextArea
            className="font-mono"
            rows={2}
            aria-label="Connection headers"
            placeholder="Headers, Name: value one per line"
            value={headers}
            onChange={(e) => setHeaders(e.target.value)}
          />
          {initial && (
            <p className="text-xs text-muted">
              Saved header values show as ****; leave them to keep the stored value.
            </p>
          )}
          {initial?.oauth && <p className="text-xs text-muted">Signs in with OAuth.</p>}
        </>
      )}
      <div className="flex items-center gap-2">
        <Button type="submit" variant="primary" disabled={!ready} loading={save.isPending}>
          {initial ? 'Save connection' : 'Add connection'}
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
        {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
      </div>
    </form>
  );
}

function ConnectionRow({ connection, used }: { connection: Connection; used: number }) {
  const [editing, setEditing] = useState(false);
  const remove = useDeleteConnection();
  const login = useLoginConnection();
  const signsIn = Boolean(connection.url) && Object.keys(connection.headers ?? {}).length === 0;
  if (editing) {
    return (
      <li>
        <ConnectionForm initial={connection} onDone={() => setEditing(false)} />
      </li>
    );
  }
  return (
    <li className="py-3">
      <div className="flex items-center gap-3">
        <span className="min-w-0 flex-1">
          <span className="block font-mono text-[13px]">{connection.name}</span>
          <span className="block truncate text-xs text-muted">
            {connection.url ? 'HTTP' : 'Local process'} · {connectionLabel(connection)}
          </span>
        </span>
        <span className="shrink-0 text-xs text-muted">
          {used} {used === 1 ? 'Bot' : 'Bots'}
        </span>
        {signsIn && (
          <Button
            size="sm"
            aria-label={`Sign in to ${connection.name}`}
            loading={login.isPending}
            onClick={() => login.mutate(connection.id)}
          >
            {login.isPending ? 'Finish in browser…' : 'Sign in'}
          </Button>
        )}
        <Button size="sm" aria-label={`Edit ${connection.name}`} onClick={() => setEditing(true)}>
          Edit
        </Button>
        <Button
          size="iconSm"
          variant="ghost"
          aria-label={`Delete ${connection.name}`}
          loading={remove.isPending}
          onClick={() => remove.mutate(connection.id)}
        >
          <svg
            className="size-3.5"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" />
          </svg>
        </Button>
      </div>
      {remove.isError && <p className="mt-1 text-sm text-danger">{remove.error.message}</p>}
      {login.isError && <p className="mt-1 text-sm text-danger">{login.error.message}</p>}
      {login.data?.map((r) => (
        <p key={r.backend} className={`mt-1 text-xs ${r.ok ? 'text-muted' : 'text-danger'}`}>
          {r.ok
            ? `Signed in for ${backendLabel(r.backend)}`
            : `${backendLabel(r.backend)}: ${r.output.split('\n').at(-1)}`}
        </p>
      ))}
    </li>
  );
}

export function ConnectionLibrary({ projectId }: { projectId?: number }) {
  const connections = useConnections();
  const bots = useBots();
  const [adding, setAdding] = useState(false);
  return (
    <div className="flex flex-col gap-3">
      {connections.isError && <p className="text-sm text-danger">{connections.error.message}</p>}
      {connections.data?.length === 0 && <p className="text-sm text-muted">No connections yet.</p>}
      <ul className="flex flex-col divide-y divide-hairline">
        {connections.data?.map((c) => (
          <ConnectionRow
            key={c.id}
            connection={c}
            used={bots.data?.filter((b) => b.connections.includes(c.id)).length ?? 0}
          />
        ))}
      </ul>
      {adding ? (
        <ConnectionForm onDone={() => setAdding(false)} />
      ) : (
        <div>
          <Button size="sm" onClick={() => setAdding(true)}>
            Add connection
          </Button>
        </div>
      )}
      <AvailableConnections projectId={projectId} />
      <p className="text-xs text-muted">
        A connection listens for nothing and sends nothing on its own. Changes reach new chats, and
        open chats after Apply here.
      </p>
    </div>
  );
}
