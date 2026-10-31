import { useState } from 'react';
import { Link } from 'react-router';
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

export function BotPanel({ bot, skills, connections, providers, noProject, onClose }: Props) {
  const [role, setRole] = useState<Role>({ ...bot, botId: bot.id, lead: false });
  const save = useUpdateBot();
  const revoke = useRevokeBot();
  const [confirming, setConfirming] = useState(false);

  return (
    <RolePanel
      role={role}
      skills={skills}
      connections={connections}
      providers={providers}
      noProject={noProject}
      onChange={setRole}
      onClose={onClose}
    >
      <div className="flex items-center gap-3">
        <Button
          variant="primary"
          type="button"
          loading={save.isPending}
          onClick={() => save.mutate({ id: bot.id, role })}
        >
          Save Bot
        </Button>
        {save.isSuccess && <span className="text-sm text-muted">Saved</span>}
        <Link to={`/bots/${bot.id}`} className="ml-auto text-sm text-soft">
          Open profile
        </Link>
      </div>
      <p className="mt-3 text-xs text-muted">Changes apply to new chats and chats you Refresh.</p>
      {save.isError && <p className="mt-3 text-sm text-danger">{save.error.message}</p>}
      <div className="mt-4 flex items-center gap-3">
        <Button variant="destructive" type="button" onClick={() => setConfirming(true)}>
          Revoke now
        </Button>
        {revoke.data && (
          <span className="text-sm text-muted">
            Revoked {revoke.data.sessions} {revoke.data.sessions === 1 ? 'session' : 'sessions'}
          </span>
        )}
      </div>
      {revoke.isError && <p className="mt-3 text-sm text-danger">{revoke.error.message}</p>}
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
    </RolePanel>
  );
}
