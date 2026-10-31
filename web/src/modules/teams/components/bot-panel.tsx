import { useState } from 'react';
import { Link } from 'react-router';
import { useUpdateBot } from '@/modules/teams/api/update-bot';
import { RolePanel } from '@/modules/teams/components/role-panel';
import type { Connection, Provider, Role, Skills, Bot } from '@/types/api';
import { Button } from '@/components/button';

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
    </RolePanel>
  );
}
