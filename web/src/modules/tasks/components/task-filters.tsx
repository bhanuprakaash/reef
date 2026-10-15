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
      </Select>
      <Select
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
      </Select>
      <Select
        aria-label="Filter by status"
        value={status}
        onChange={(e) => onChange('status', e.target.value)}
      >
        <option value="">Open and done</option>
        <option value="open">Open</option>
        <option value="done">Done</option>
      </Select>
    </div>
  );
}
