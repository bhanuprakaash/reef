import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, expect, it, vi } from 'vitest';
import type { Project } from '@/types/api';
import { useUpdateProject } from './update-project';

const project: Project = {
  id: 1,
  name: 'shop',
  path: '/work/shop',
  baseBranch: '',
  purpose: '',
  icon: '',
  colour: '',
  createdAt: '',
};

afterEach(() => vi.unstubAllGlobals());

it('sends each save on top of the one before it', async () => {
  const bodies: Project[] = [];
  let stored = project;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, init: RequestInit) => {
      if (init.method === 'PUT') {
        stored = { ...stored, ...JSON.parse(init.body as string) };
        bodies.push(stored);
        return new Response(JSON.stringify(stored));
      }
      return new Response(JSON.stringify([stored]));
    }),
  );
  const client = new QueryClient();
  client.setQueryData(['projects'], [project]);
  let update!: ReturnType<typeof useUpdateProject>;
  function Probe() {
    update = useUpdateProject(project);
    return null;
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <Probe />
    </QueryClientProvider>,
  );

  await Promise.all([
    update.mutateAsync({ icon: '🚀' }),
    update.mutateAsync({ colour: '#2F72C0' }),
  ]);
  await update.mutateAsync({ purpose: 'Sells things.' });

  expect(bodies.map(({ icon, colour, purpose }) => ({ icon, colour, purpose }))).toEqual([
    { icon: '🚀', colour: '', purpose: '' },
    { icon: '🚀', colour: '#2F72C0', purpose: '' },
    { icon: '🚀', colour: '#2F72C0', purpose: 'Sells things.' },
  ]);
});
