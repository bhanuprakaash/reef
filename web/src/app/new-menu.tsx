import { useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { Dialog } from '@/components/dialog';
import { Menu, MenuItem } from '@/components/menu';
import { useDismiss } from '@/lib/use-dismiss';
import { AddFeatureForm } from '@/modules/features/components/add-feature-form';
import { NewConversationForm } from '@/modules/spaces/components/new-conversation-form';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import type { Project } from '@/types/api';

type Kind = 'work-item' | 'space' | 'conversation';

const TITLES: Record<Kind, string> = {
  'work-item': 'New Work item',
  space: 'New Space',
  conversation: 'New conversation',
};

export function NewMenu({ project }: { project: Project | undefined }) {
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState<Kind | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const close = () => setCreating(null);

  useDismiss(open, ref, () => setOpen(false));

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        aria-label="New"
        aria-expanded={open}
        disabled={!project}
        onClick={() => setOpen(!open)}
        className="flex size-8 items-center justify-center rounded-lg text-soft hover:bg-hover/50"
      >
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <path d="M12 4v16M4 12h16" />
        </svg>
      </button>
      {open && (
        <Menu className="absolute top-full right-0 z-10 mt-1 w-48">
          {(Object.keys(TITLES) as Kind[]).map((kind) => (
            <MenuItem
              key={kind}
              onClick={() => {
                setCreating(kind);
                setOpen(false);
              }}
            >
              {TITLES[kind]}
            </MenuItem>
          ))}
        </Menu>
      )}
      {project && (
        <Dialog open={creating !== null} onClose={close} title={TITLES[creating ?? 'space']}>
          {creating === 'work-item' && (
            <AddFeatureForm
              projectId={project.id}
              onClose={close}
              onCreated={(item) => void navigate(`/features/${item.id}`)}
            />
          )}
          {creating === 'space' && (
            <NewSpaceForm projectId={project.id} featureId={null} onClose={close} />
          )}
          {creating === 'conversation' && (
            <NewConversationForm projectId={project.id} onClose={close} />
          )}
        </Dialog>
      )}
    </div>
  );
}
