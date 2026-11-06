import { useState } from 'react';
import { Link } from 'react-router';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';
import { useRepoConnections } from '@/lib/api/get-repo-connections';
import { useBindRemote, useDetectedRemote } from '@/modules/projects/api/project-remote';
import type { Project } from '@/types/api';

export function ProjectRemoteForm({ project }: { project: Project }) {
  const connections = useRepoConnections();
  const detected = useDetectedRemote(project.id);
  const bind = useBindRemote(project.id);
  const [draft, setDraft] = useState<{ connectionId?: string; path?: string }>({});

  const list = connections.data ?? [];
  const connectionId =
    draft.connectionId ?? String(project.repoConnectionId ?? detected.data?.connectionId ?? '');
  const path = draft.path ?? (project.remotePath || detected.data?.path || '');
  const chosen = list.find((c) => String(c.id) === connectionId);
  const unchanged =
    String(project.repoConnectionId ?? '') === connectionId && project.remotePath === path.trim();

  return (
    <div>
      <div className="mb-1.5 flex items-center text-xs text-muted">
        <label htmlFor="project-remote-path">Repository</label>
        {bind.isSuccess && unchanged && <span className="ml-auto text-done">Saved</span>}
      </div>
      {list.length === 0 ? (
        <p className="text-[12.5px] text-muted">
          Connect GitHub or GitLab in{' '}
          <Link className="underline" to="/settings?section=repositories">
            Settings
          </Link>{' '}
          to bind this Project.
        </p>
      ) : (
        <div className="flex flex-col gap-2">
          <Select
            aria-label="Repository connection"
            value={connectionId}
            onChange={(value) => setDraft({ ...draft, connectionId: value })}
            options={list.map((c) => ({
              value: String(c.id),
              label: `${c.host}${c.account ? ` · ${c.account}` : ''}`,
              hint: c.state === 'connected' ? undefined : 'disconnected',
            }))}
          />
          <TextField
            id="project-remote-path"
            className="w-full font-mono"
            placeholder="owner/repo"
            value={path}
            onChange={(e) => setDraft({ ...draft, path: e.target.value })}
          />
          <Button
            size="sm"
            className="self-start"
            loading={bind.isPending}
            disabled={!chosen || path.trim() === '' || unchanged}
            onClick={() =>
              chosen &&
              bind.mutate(
                { connectionId: chosen.id, path: path.trim() },
                { onSuccess: () => setDraft({}) },
              )
            }
          >
            Bind
          </Button>
        </div>
      )}
      {chosen?.state === 'disconnected' && (
        <p className="mt-1.5 text-[12.5px] text-danger">
          {chosen.host} is disconnected: {chosen.reason}
        </p>
      )}
      <p className="mt-1.5 text-[12.5px] text-muted">
        {detected.data
          ? `Detected from origin: ${detected.data.host}/${detected.data.path}.`
          : detected.error?.message}
      </p>
      {bind.isError && <p className="mt-1.5 text-sm text-danger">{bind.error.message}</p>}
    </div>
  );
}
