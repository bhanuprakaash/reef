import type { Feature, Project } from '@/types/api';
import { Select } from '@/components/select';

type Props = {
  projects: Project[];
  features: Feature[];
  project: string;
  feature: string;
  status: string;
  onChange: (name: 'project' | 'feature' | 'status', value: string) => void;
};

export function TaskFilters({ projects, features, project, feature, status, onChange }: Props) {
  const shownFeatures =
    project === '' ? features : features.filter((f) => String(f.projectId) === project);

  return (
    <div className="flex flex-wrap gap-2">
      <Select
        variant="pill"
        aria-label="Filter by project"
        value={project}
        onChange={(v) => onChange('project', v)}
        options={[
          { value: '', label: 'All projects' },
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
