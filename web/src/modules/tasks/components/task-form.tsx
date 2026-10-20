import { useState } from 'react';
import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Feature, Task, TaskStatus } from '@/types/api';
import { Button } from '@/components/button';
import { SegmentedControl } from '@/components/segmented-control';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

type Props = { task: Task; features: Feature[]; onDone: () => void };

export function TaskForm({ task, features, onDone }: Props) {
  const [title, setTitle] = useState(task.title);
  const [body, setBody] = useState(task.body);
  const [status, setStatus] = useState<TaskStatus>(task.status);
  const [featureId, setFeatureId] = useState(task.featureId === null ? '' : String(task.featureId));
  const update = useUpdateTask();

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate(
          { task, title, body, status, featureId: featureId === '' ? null : Number(featureId) },
          { onSuccess: onDone },
        );
      }}
    >
      <label className="block space-y-1">
        <span className="text-sm text-muted">Title</span>
        <TextField
          className="w-full"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
      </label>
      <label className="block space-y-1">
        <span className="text-sm text-muted">Details</span>
        <TextArea
          className="w-full"
          rows={6}
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
      </label>
      <div className="space-y-1">
        <span className="text-sm text-muted">Status</span>
        <div>
          <SegmentedControl
            label="Status"
            options={[
              { value: 'todo', label: 'To do' },
              { value: 'doing', label: 'Doing' },
              { value: 'done', label: 'Done' },
            ]}
            value={status}
            onChange={setStatus}
          />
        </div>
      </div>
      <div className="space-y-1">
        <span className="text-sm text-muted">Feature</span>
        <Select
          className="w-full"
          aria-label="Feature"
          value={featureId}
          onChange={setFeatureId}
          options={[
            { value: '', label: 'No feature' },
            ...features
              .filter((f) => f.projectId === task.projectId)
              .map((f) => ({ value: String(f.id), label: f.name })),
          ]}
        />
      </div>
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onDone}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" loading={update.isPending}>
          Save
        </Button>
      </div>
    </form>
  );
}
