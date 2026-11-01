import { useState } from 'react';
import { useApproveWorkItem } from '@/modules/work-items/api/approve-work-item';
import type { WorkItem } from '@/types/api';
import { useBots } from '@/lib/api/get-bots';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Select } from '@/components/select';

// The key is made once when the card opens, so a double click or a retry is the same approval.
export function ApproveCard({ item }: { item: WorkItem }) {
  const [key] = useState(() => crypto.randomUUID());
  const [ownerBotId, setOwnerBotId] = useState(item.ownerBotId);
  const [participantBotIds, setParticipantBotIds] = useState(item.participantBotIds);
  const [dropped, setDropped] = useState<number[]>([]);
  const bots = useBots();
  const approve = useApproveWorkItem(item.id);

  return (
    <Card title="Approve">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (ownerBotId === null) return;
          approve.mutate({
            ownerBotId,
            participantBotIds,
            targetProjectIds: item.targets
              .map((t) => t.projectId)
              .filter((id) => !dropped.includes(id)),
            key,
          });
        }}
      >
        <p className="text-[13px] text-muted">
          {item.proposedBy ? `Proposed by ${item.proposedBy}. ` : ''}
          Approving authorizes this owner, these participants and these repositories, and queues the
          item.
        </p>
        <div className="space-y-1">
          <span className="text-sm text-muted">Owner</span>
          <Select
            className="w-full"
            aria-label="Owner"
            value={ownerBotId === null ? '' : String(ownerBotId)}
            onChange={(v) => setOwnerBotId(v === '' ? null : Number(v))}
            options={[
              { value: '', label: 'Choose an owner' },
              ...(bots.data ?? []).map((b) => ({ value: String(b.id), label: b.name })),
            ]}
          />
        </div>
        <div className="space-y-1">
          <span className="text-sm text-muted">Participants</span>
          <ul className="space-y-1">
            {(bots.data ?? []).map((b) => (
              <li key={b.id} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  aria-label={`Include ${b.name}`}
                  checked={b.id === ownerBotId || participantBotIds.includes(b.id)}
                  disabled={b.id === ownerBotId}
                  onChange={(e) =>
                    setParticipantBotIds(
                      e.target.checked
                        ? [...participantBotIds, b.id]
                        : participantBotIds.filter((id) => id !== b.id),
                    )
                  }
                />
                {b.name}
              </li>
            ))}
          </ul>
        </div>
        <div className="space-y-1">
          <span className="text-sm text-muted">Repositories</span>
          <ul className="space-y-1">
            {item.targets.map((t) => (
              <li key={t.projectId} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  aria-label={`Repository ${t.projectName}`}
                  checked={t.projectId === item.projectId || !dropped.includes(t.projectId)}
                  disabled={t.projectId === item.projectId}
                  onChange={(e) =>
                    setDropped(
                      e.target.checked
                        ? dropped.filter((id) => id !== t.projectId)
                        : [...dropped, t.projectId],
                    )
                  }
                />
                {t.projectName}
                {t.authorizedBy === '' && t.proposedBy !== '' && (
                  <span className="text-xs text-muted">proposed by {t.proposedBy}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
        {approve.isError && <p className="text-sm text-danger">{approve.error.message}</p>}
        <Button
          variant="primary"
          type="submit"
          disabled={ownerBotId === null}
          loading={approve.isPending}
        >
          Approve
        </Button>
      </form>
    </Card>
  );
}
