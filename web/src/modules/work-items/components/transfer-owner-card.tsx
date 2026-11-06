import { useState } from 'react';
import { useTransferOwner } from '@/modules/work-items/api/transfer-owner';
import type { WorkItem } from '@/types/api';
import { useBots } from '@/lib/api/get-bots';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Select } from '@/components/select';

export function TransferOwnerCard({ item }: { item: WorkItem }) {
  const bots = useBots();
  const transfer = useTransferOwner(item.id);
  const [botId, setBotId] = useState('');
  const all = bots.data ?? [];
  const name = (id: number | null) => all.find((b) => b.id === id)?.name ?? 'nobody';
  const others = all.filter((b) => b.id !== item.ownerBotId);
  const inItem = others.filter((b) => item.participantBotIds.includes(b.id));
  const outside = others.filter((b) => !item.participantBotIds.includes(b.id));

  return (
    <Card title="Owner">
      <p className="text-[13px] text-muted">
        {name(item.ownerBotId)} owns this Work item.
        {item.pendingOwnerBotId !== null &&
          ` Transfer to ${name(item.pendingOwnerBotId)} applies after the current turn. Stop the turn to apply it now.`}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Select
          className="min-w-[160px] flex-1"
          aria-label="New owner"
          value={botId}
          onChange={setBotId}
          options={[
            { value: '', label: 'Choose a Bot' },
            ...inItem.map((b) => ({ value: String(b.id), label: `${b.name} (participant)` })),
            ...outside.map((b) => ({ value: String(b.id), label: b.name })),
          ]}
        />
        <Button
          type="button"
          disabled={botId === ''}
          loading={transfer.isPending}
          onClick={() => transfer.mutate(Number(botId), { onSuccess: () => setBotId('') })}
        >
          Transfer ownership
        </Button>
      </div>
      {transfer.isError && <p className="mt-2 text-sm text-danger">{transfer.error.message}</p>}
    </Card>
  );
}
