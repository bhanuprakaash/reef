import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { TextField } from '@/components/text-field';
import { useProfile } from '@/lib/api/get-profile';
import { usePutProfile } from '@/lib/api/put-profile';
import { useCreateProject } from '@/modules/projects/api/create-project';
import { useProjects } from '@/modules/projects/api/get-projects';

function NameStep() {
  const [name, setName] = useState('');
  const save = usePutProfile();

  return (
    <form
      className="flex w-full flex-col gap-3 rounded-[22px] bg-card p-[18px] text-left"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(name);
      }}
    >
      <div className="space-y-1.5">
        <label htmlFor="first-run-your-name" className="block text-[12.5px] text-muted">
          Your name
        </label>
        <TextField
          id="first-run-your-name"
          className="w-full"
          placeholder="Ada"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          required
        />
      </div>
      {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
      <Button variant="primary" size="lg" type="submit" loading={save.isPending}>
        Continue
      </Button>
    </form>
  );
}

function ProjectStep() {
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const create = useCreateProject();
  const navigate = useNavigate();

  return (
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
  );
}

export function FirstRun() {
  const profile = useProfile();
  const projects = useProjects();
  const askName = !profile.data;
  const twoSteps = projects.data?.length === 0;

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
        {twoSteps && <p className="text-[13px] text-muted">Step {askName ? 1 : 2} of 2</p>}
        <h1 className="text-4xl font-normal tracking-[-0.02em]">
          {askName ? 'What should the team call you?' : 'Welcome to the reef'}
        </h1>
        <p className="text-base text-muted">
          {askName
            ? 'The team and the sidebar use it to greet you.'
            : 'Point Reef at a repo. Every feature you start there keeps its own memory, and a team works on it with you.'}
        </p>
        {askName ? <NameStep /> : <ProjectStep />}
        <p className="text-[13px] text-muted">
          Reef keeps its data in its own folder, never in your repo.
        </p>
      </div>
    </div>
  );
}
