import { useProjects } from '@/modules/projects/api/get-projects';
import { ProjectMemoryForm } from '@/modules/projects/components/project-memory-form';
import { errorClass, mutedClass } from '@/utils/classes';

export function HomePage() {
  const projects = useProjects();

  return (
    <div className="max-w-3xl space-y-8">
      <h1 className="text-xl font-semibold">Home</h1>
      {projects.isPending && <p className={mutedClass}>Loading...</p>}
      {projects.isError && <p className={errorClass}>{projects.error.message}</p>}
      {projects.data?.map((p) => (
        <section key={p.id} className="space-y-2">
          <h2 className="text-lg font-medium">{p.name}</h2>
          <ProjectMemoryForm projectId={p.id} />
        </section>
      ))}
    </div>
  );
}
