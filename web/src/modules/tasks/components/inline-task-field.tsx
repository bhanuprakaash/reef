import { useEffect, useRef, useState } from 'react';
import type { ChangeEvent, KeyboardEvent } from 'react';
import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Task } from '@/types/api';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

type Props = {
  task: Task;
  field: 'title' | 'body';
  className?: string;
  multiline?: boolean;
};

export function InlineTaskField({ task, field, className = '', multiline = false }: Props) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(task[field]);
  const input = useRef<HTMLInputElement | HTMLTextAreaElement>(null);
  const finished = useRef(false);
  const update = useUpdateTask();

  useEffect(() => {
    if (editing) input.current?.focus();
  }, [editing]);

  function cancel() {
    finished.current = true;
    setValue(task[field]);
    setEditing(false);
  }

  function save() {
    if (finished.current) return;
    finished.current = true;
    if ((field === 'title' && value.trim() === '') || value === task[field]) {
      setValue(task[field]);
      setEditing(false);
      return;
    }
    update.mutate(
      { task, [field]: value },
      {
        onSuccess: () => setEditing(false),
        onError: () => {
          finished.current = false;
        },
      },
    );
  }

  if (editing) {
    const props = {
      'aria-label': field === 'title' ? 'Task title' : 'Task details',
      className: `w-full ${className}`,
      value,
      disabled: update.isPending,
      onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
        setValue(event.target.value),
      onBlur: save,
      onKeyDown: (event: KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        if (event.key === 'Escape') {
          event.preventDefault();
          cancel();
        } else if (!multiline && event.key === 'Enter') {
          event.preventDefault();
          save();
        }
      },
    };
    const ref = (element: HTMLInputElement | HTMLTextAreaElement | null) => {
      input.current = element;
    };
    return (
      <>
        {multiline ? (
          <TextArea {...props} ref={ref} rows={3} />
        ) : (
          <TextField {...props} ref={ref} />
        )}
        {update.isError && <p className="text-xs text-danger">{update.error.message}</p>}
      </>
    );
  }

  return (
    <button
      type="button"
      className={`min-h-11 min-w-0 text-left md:min-h-0 ${className}`}
      aria-label={`Edit task ${field === 'title' ? 'title' : 'details'}`}
      onClick={() => {
        finished.current = false;
        setValue(task[field]);
        setEditing(true);
      }}
    >
      {task[field] || (field === 'body' ? 'No details yet.' : 'Untitled task')}
    </button>
  );
}
