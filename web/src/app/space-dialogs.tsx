import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { TextField } from '@/components/text-field';
import { useDeleteSpace } from '@/modules/spaces/api/delete-space';
import { useRenameSpace } from '@/modules/spaces/api/rename-space';
import type { SpacePreview } from '@/types/api';

type Props = { space: SpacePreview; open: boolean; onClose: () => void };

function RenameForm({ space, onClose }: Omit<Props, 'open'>) {
  const [name, setName] = useState(space.name);
  const rename = useRenameSpace(space.id);
  const valid = name.trim() !== '';

  return (
    <form
      className="space-y-3.5"
      onSubmit={(e) => {
        e.preventDefault();
        rename.mutate(name.trim(), { onSuccess: onClose });
      }}
    >
      <TextField
        className="w-full"
        aria-label="Name"
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      {rename.isError && <p className="text-sm text-danger">{rename.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" disabled={!valid} loading={rename.isPending}>
          Rename
        </Button>
      </div>
    </form>
  );
}

export function RenameSpaceDialog({ space, open, onClose }: Props) {
  return (
    <Dialog open={open} onClose={onClose} title="Rename">
      {open && <RenameForm space={space} onClose={onClose} />}
    </Dialog>
  );
}

function DeleteForm({ space, onClose }: Omit<Props, 'open'>) {
  const remove = useDeleteSpace(space.id);
  const { pathname } = useLocation();
  const navigate = useNavigate();

  return (
    <div className="space-y-3.5">
      <p className="text-sm text-soft">
        Delete {space.name} and its messages? This cannot be undone.
      </p>
      {remove.isError && <p className="text-sm text-danger">{remove.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="destructive"
          loading={remove.isPending}
          onClick={() =>
            remove.mutate(undefined, {
              onSuccess: () => {
                onClose();
                if (pathname === `/spaces/${space.id}`) void navigate('/');
              },
            })
          }
        >
          Delete
        </Button>
      </div>
    </div>
  );
}

export function DeleteSpaceDialog({ space, open, onClose }: Props) {
  return (
    <Dialog open={open} onClose={onClose} title="Delete">
      {open && <DeleteForm space={space} onClose={onClose} />}
    </Dialog>
  );
}
