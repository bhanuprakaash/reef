import { ResourceRow } from '@/app/resource-row';
import { useFind } from '@/lib/api/find';

export function SidebarSearch({ query }: { query: string }) {
  const found = useFind(query);
  return (
    <section aria-label="Search results" className="flex flex-col gap-0.5 pt-2">
      {found.isError && <p className="px-2.5 text-sm text-danger">{found.error.message}</p>}
      {found.data?.length === 0 && <p className="px-2.5 text-[13px] text-faint">No results</p>}
      <ul className="flex flex-col gap-0.5">
        {found.data?.map((r) => (
          <li key={`${r.kind}-${r.id}`}>
            <ResourceRow resource={r} />
          </li>
        ))}
      </ul>
    </section>
  );
}
