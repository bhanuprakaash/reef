import { Link } from 'react-router';
import { useProjects } from '@/modules/projects/api/get-projects';

export function Sidebar() {
  const projects = useProjects();

  return (
    <nav className="space-y-4 text-sm">
      <Link to="/tasks" className="block hover:underline">
        Tasks
      </Link>
      <div>
        <h2 className="mb-1 text-xs font-medium uppercase text-neutral-500">Projects</h2>
        {projects.isPending && <p className="text-neutral-500">Loading...</p>}
        {projects.isError && <p className="text-red-600">{projects.error.message}</p>}
        <ul className="space-y-1">
          {projects.data?.map((p) => (
            <li key={p.id}>{p.name}</li>
          ))}
        </ul>
      </div>
    </nav>
  );
}
