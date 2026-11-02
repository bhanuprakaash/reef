import { useState } from 'react';
import { botRequestBody } from '@/lib/api/bot-request-body';
import { useDeleteBot } from '@/modules/teams/api/delete-bot';
import { useUpdateBot } from '@/modules/teams/api/update-bot';
import { useRevokeBot } from '@/modules/teams/api/revoke-bot';
import { RolePanel } from '@/modules/teams/components/role-panel';
import type { Connection, Provider, Role, Skills, Bot } from '@/types/api';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';

type Props = {
  bot: Bot;
  skills?: Skills;
  connections?: Connection[];
  providers?: Provider[];
  noProject?: boolean;
  onClose: () => void;
};

const roleOf = (bot: Bot): Role => ({ ...bot, botId: bot.id, lead: false });
const saved = (role: Role) =>
  JSON.stringify(botRequestBody(role), (_, v) => (v === '' ? undefined : v));

export function BotPanel({ bot, skills, connections, providers, noProject, onClose }: Props) {
  const [role, setRole] = useState<Role>(roleOf(bot));
  const save = useUpdateBot();
  const revoke = useRevokeBot();
  const remove = useDeleteBot();
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const dirty = saved(role) !== saved(roleOf(bot));

  return (
    <RolePanel
      role={role}
      skills={skills}
      connections={connections}
      providers={providers}
      noProject={noProject}
      onChange={setRole}
      onClose={onClose}
      extra={
        <section className="space-y-2">
          <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">Sessions</h3>
          <p className="text-xs text-muted">
            Changes apply to new chats. To restart an open chat with them, use Apply here in that
            chat's Bots panel.
          </p>
          <div className="flex items-center gap-3">
            <Button type="button" size="sm" onClick={() => setConfirming(true)}>
              Revoke now
            </Button>
            {revoke.data && (
              <span className="text-sm text-muted">
                Revoked {revoke.data.sessions} {revoke.data.sessions === 1 ? 'session' : 'sessions'}
              </span>
            )}
          </div>
          {revoke.isError && <p className="text-sm text-danger">{revoke.error.message}</p>}
        </section>
      }
    >
      <Button
        variant="ghost"
        type="button"
        className="text-danger!"
        onClick={() => setDeleting(true)}
      >
        Delete
      </Button>
      <span className="ml-auto text-xs text-muted">{dirty ? 'Unsaved changes' : ''}</span>
      <Button
        variant="primary"
        type="button"
        aria-label="Save Bot"
        loading={save.isPending}
        onClick={() => save.mutate({ id: bot.id, role })}
      >
        Save
      </Button>
      {save.isSuccess && !dirty && <span className="text-sm text-muted">Saved</span>}
      {save.isError && <p className="basis-full text-sm text-danger">{save.error.message}</p>}
      <Dialog open={confirming} onClose={() => setConfirming(false)} title={`Revoke ${bot.name}?`}>
        <p className="text-sm text-soft">
          This stops every chat and Space session of {bot.name} and deletes its Reef tokens. Chat
          history stays. The next message starts a new session with the current settings. Tool
          activity that was running may have finished or not; revoking does not undo its effects.
        </p>
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            type="button"
            loading={revoke.isPending}
            onClick={() => revoke.mutate(bot.id, { onSuccess: () => setConfirming(false) })}
          >
            Revoke now
          </Button>
        </div>
      </Dialog>
      <Dialog open={deleting} onClose={() => setDeleting(false)} title={`Delete ${bot.name}?`}>
        <p className="text-sm text-soft">
          This deletes {bot.name}. A Bot that is in a Crew has to leave it first.
        </p>
        {remove.isError && <p className="mt-3 text-sm text-danger">{remove.error.message}</p>}
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" onClick={() => setDeleting(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            type="button"
            loading={remove.isPending}
            onClick={() => remove.mutate(bot.id, { onSuccess: onClose })}
          >
            Delete
          </Button>
        </div>
      </Dialog>
    </RolePanel>
  );
}
