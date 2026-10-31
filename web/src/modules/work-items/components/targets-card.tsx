import { useState } from 'react';
import { useAddTarget } from '@/modules/work-items/api/add-target';
import type { Project, WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Chip } from '@/components/chip';
import { Select } from '@/components/select';

export function TargetsCard({ item, projects }: { item: WorkItem; projects: Project[] }) {
  const add = useAddTarget(item.id);
  const [projectId, setProjectId] = useState('');
  const open = projects.filter((p) => !item.targets.some((t) => t.projectId === p.id));

  return (
    <Card title="Repositories">
      <ul className="flex flex-wrap gap-2">
        {item.targets.map((t) => (
          <li key={t.projectId}>
            <Chip title={`Authorized by ${t.authorizedBy}`}>{t.projectName}</Chip>
          </li>
        ))}
      </ul>
      {open.length > 0 && (
        <div className="mt-3 flex items-center gap-2">
          <Select
            className="min-w-0 flex-1"
            aria-label="Repository"
            value={projectId}
            onChange={setProjectId}
            options={[
              { value: '', label: 'Choose a Project' },
              ...open.map((p) => ({ value: String(p.id), label: p.name })),
            ]}
          />
          <Button
            type="button"
            disabled={projectId === ''}
            loading={add.isPending}
            onClick={() => add.mutate(Number(projectId), { onSuccess: () => setProjectId('') })}
          >
            Add repository
          </Button>
        </div>
      )}
      {add.isError && <p className="mt-2 text-sm text-danger">{add.error.message}</p>}
    </Card>
  );
}
