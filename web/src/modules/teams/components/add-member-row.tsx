import { useState } from 'react';
import { Link } from 'react-router';
import { useTeammates } from '@/lib/api/get-teammates';
import { useAddMember } from '@/modules/teams/api/add-member';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import type { Role } from '@/types/api';
import { Creature } from '@/components/creature';
import { Dialog } from '@/components/dialog';

type Props = { team: string; members: Role[]; onAdded: (role: Role) => void };

export function AddMemberRow({ team, members, onAdded }: Props) {
  const [open, setOpen] = useState(false);
  const teammates = useTeammates();
  const add = useAddMember();
  const close = () => setOpen(false);
  const available = (teammates.data ?? []).filter(
    (t) => !members.some((m) => m.teammateId === t.id),
  );

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex h-12 w-full items-center justify-center gap-2 rounded-2xl border border-dashed border-faint text-sm text-soft outline-none hover:bg-card focus-visible:ring-3 focus-visible:ring-ring"
      >
        <span aria-hidden="true">+</span> Add a teammate
      </button>
      <Dialog open={open} onClose={close} title="Add a teammate">
        {open && (
          <div className="space-y-1.5">
            {available.map((t) => (
              <button
                key={t.id}
                type="button"
                disabled={add.isPending}
                onClick={() =>
                  add.mutate(
                    { team, teammateId: t.id },
                    {
                      onSuccess: (role) => {
                        onAdded(role);
                        close();
                      },
                    },
                  )
                }
                className="flex min-h-12 w-full items-center gap-3 rounded-xl border border-transparent p-2 text-left outline-none hover:bg-bubble focus-visible:ring-3 focus-visible:ring-ring"
              >
                <Creature look={t} size={32} />
                <span className="min-w-0 flex-1 truncate text-sm">{t.name}</span>
                <span className="text-xs text-muted">{backendLabel(t.backend)}</span>
              </button>
            ))}
            {available.length === 0 && (
              <p className="text-sm text-muted">
                Every teammate is in this team. Create more on the{' '}
                <Link to="/teammates" className="underline">
                  Teammates page
                </Link>
                .
              </p>
            )}
          </div>
        )}
        {add.isError && <p className="mt-3 text-sm text-danger">{add.error.message}</p>}
      </Dialog>
    </>
  );
}
