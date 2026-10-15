import type { Feature, Project } from '@/types/api';

type Props = {
  projects: Project[];
  features: Feature[];
  project: string;
  feature: string;
  status: string;
  onChange: (name: 'project' | 'feature' | 'status', value: string) => void;
};

const selectClass =
  'h-11 rounded-pill bg-bubble px-3 text-[13px] text-fg outline-none focus-visible:ring-3 focus-visible:ring-ring md:h-[30px]';

export function TaskFilters({ projects, features, project, feature, status, onChange }: Props) {
  const shownFeatures =
    project === '' ? features : features.filter((f) => String(f.projectId) === project);

  return (
    <div className="flex flex-wrap gap-2">
      <select
        className={selectClass}
        aria-label="Filter by project"
        value={project}
        onChange={(e) => onChange('project', e.target.value)}
      >
        <option value="">All projects</option>
        {projects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <select
        className={selectClass}
        aria-label="Filter by feature"
        value={feature}
        onChange={(e) => onChange('feature', e.target.value)}
      >
        <option value="">All features</option>
        {shownFeatures.map((f) => (
          <option key={f.id} value={f.id}>
            {f.name}
          </option>
        ))}
      </select>
      {(
        [
          ['open', 'Open'],
          ['done', 'Done'],
        ] as const
      ).map(([value, label]) => (
        <button
          key={value}
          type="button"
          aria-pressed={status === value}
          onClick={() => onChange('status', status === value ? '' : value)}
          className={`h-11 rounded-pill px-3 md:h-[30px] text-[13px] ${status === value ? 'bg-pill text-pill-ink' : 'border border-line text-muted'}`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
