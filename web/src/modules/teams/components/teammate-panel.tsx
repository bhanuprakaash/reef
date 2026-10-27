import { useState } from 'react';
import { Link } from 'react-router';
import { useUpdateTeammate } from '@/modules/teams/api/update-teammate';
import { RolePanel } from '@/modules/teams/components/role-panel';
import type { Provider, Role, Skills, Teammate } from '@/types/api';
import { Button } from '@/components/button';

type Props = {
  teammate: Teammate;
  skills?: Skills;
  providers?: Provider[];
  noProject?: boolean;
  onClose: () => void;
};

export function TeammatePanel({ teammate, skills, providers, noProject, onClose }: Props) {
  const [role, setRole] = useState<Role>({ ...teammate, teammateId: teammate.id, lead: false });
  const save = useUpdateTeammate();

  return (
    <RolePanel
      scope="teammate"
      role={role}
      skills={skills}
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
          onClick={() => save.mutate({ id: teammate.id, role })}
        >
          Save teammate
        </Button>
        {save.isSuccess && <span className="text-sm text-muted">Saved</span>}
        <Link to={`/teammates/${teammate.id}`} className="ml-auto text-sm text-soft">
          Open profile
        </Link>
      </div>
      {save.isError && <p className="mt-3 text-sm text-danger">{save.error.message}</p>}
    </RolePanel>
  );
}
