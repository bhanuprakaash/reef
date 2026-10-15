import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { TextField } from '@/components/text-field';
import { useCreateProject } from '@/modules/projects/api/create-project';

export function FirstRun() {
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const create = useCreateProject();
  const navigate = useNavigate();

  return (
    <div className="flex h-screen items-center justify-center overflow-y-auto bg-main px-4 py-10 font-sans text-fg">
      <div className="app-drag fixed inset-x-0 top-0 h-8" />
      <div className="flex w-full max-w-[520px] flex-col items-center gap-[22px] text-center">
        <div className="flex items-end gap-2.5">
          <Creature role="architect" size={44} />
          <Creature role="lead" size={72} />
          <Creature role="developer" size={44} />
          <Creature role="tester" size={44} />
        </div>
        <h1 className="text-4xl font-normal tracking-[-0.02em]">Welcome to the reef</h1>
        <p className="text-base text-muted">
          Point Reef at a repo. Every feature you start there keeps its own memory, and a team works
          on it with you.
        </p>
        <form
          className="flex w-full flex-col gap-3 rounded-[22px] bg-card p-[18px] text-left"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate({ name, path }, { onSuccess: () => navigate('/') });
          }}
        >
          <div className="space-y-1.5">
            <label htmlFor="first-run-path" className="block text-[12.5px] text-muted">
              Repo folder
            </label>
            <TextField
              id="first-run-path"
              className="w-full font-mono"
              placeholder="/Users/you/code/project"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              required
            />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="first-run-name" className="block text-[12.5px] text-muted">
              Name
            </label>
            <TextField
              id="first-run-name"
              className="w-full"
              placeholder="project"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
          <Button variant="primary" size="lg" type="submit" loading={create.isPending}>
            Add project
          </Button>
        </form>
        <p className="text-[13px] text-muted">
          Reef keeps its data in its own folder, never in your repo.
        </p>
      </div>
    </div>
  );
}
