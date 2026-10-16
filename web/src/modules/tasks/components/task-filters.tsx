import type { Feature, Project } from '@/types/api';
import { SegmentedControl } from '@/components/segmented-control';
import { Select } from '@/components/select';

type Props = {
  projects: Project[];
  features: Feature[];
  project: string;
  feature: string;
  status: string;
  view: 'list' | 'board';
  onChange: (name: 'project' | 'feature' | 'status', value: string) => void;
  onView: (view: 'list' | 'board') => void;
};

export function TaskFilters({
  projects,
  features,
  project,
  feature,
  status,
  view,
  onChange,
  onView,
}: Props) {
  const shownFeatures =
    project === 'all' ? features : features.filter((f) => String(f.projectId) === project);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        variant="pill"
        aria-label="Filter by project"
        value={project}
        onChange={(v) => onChange('project', v)}
        options={[
          { value: 'all', label: 'All projects' },
          ...projects.map((p) => ({ value: String(p.id), label: p.name })),
        ]}
      />
      <Select
        variant="pill"
        aria-label="Filter by feature"
        value={feature}
        onChange={(v) => onChange('feature', v)}
        options={[
          { value: '', label: 'All features' },
          ...shownFeatures.map((f) => ({ value: String(f.id), label: f.name })),
        ]}
      />
      {view === 'list' && (
        <SegmentedControl
          label="Status"
          options={[
            { value: 'open', label: 'Open' },
            { value: 'done', label: 'Done' },
            { value: 'all', label: 'All' },
          ]}
          value={status}
          onChange={(v) => onChange('status', v === 'open' ? '' : v)}
        />
      )}
      <SegmentedControl
        className="ml-auto"
        label="View"
        options={[
          { value: 'list', label: 'List' },
          { value: 'board', label: 'Board' },
        ]}
        value={view}
        onChange={onView}
      />
    </div>
  );
}
