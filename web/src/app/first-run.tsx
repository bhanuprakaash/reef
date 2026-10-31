import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Button } from '@/components/button';
import { ProfileFields } from '@/app/profile-fields';
import { Creature } from '@/components/creature';
import { TextField } from '@/components/text-field';
import { useProfile } from '@/lib/api/get-profile';
import { usePutProfile } from '@/lib/api/put-profile';
import { useCreateProject } from '@/modules/projects/api/create-project';
import { useProjects } from '@/modules/projects/api/get-projects';

function NameStep() {
  const [name, setName] = useState('');
  const [occupation, setOccupation] = useState('');
  const [style, setStyle] = useState<string[]>([]);
  const save = usePutProfile();

  return (
    <form
      className="flex w-full flex-col gap-3 rounded-[22px] bg-card p-[18px] text-left"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate({ name, occupation, style });
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
      <ProfileFields
        occupation={occupation}
        style={style}
        onOccupation={setOccupation}
        onStyle={setStyle}
      />
      {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
      <div className="flex justify-end">
        <Button variant="primary" type="submit" loading={save.isPending}>
          Continue
        </Button>
      </div>
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
        <div className="flex gap-2">
          <TextField
            id="first-run-path"
            className="min-w-0 flex-1 font-mono"
            placeholder="/Users/you/code/project"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            required
          />
          {window.reef && (
            <Button
              type="button"
              onClick={() => void window.reef?.chooseFolder().then((p) => p && setPath(p))}
            >
              Choose…
            </Button>
          )}
        </div>
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
      <div className="flex justify-end">
        <Button variant="primary" type="submit" loading={create.isPending}>
          Add project
        </Button>
      </div>
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
          <Creature look={{ face: 'whale' }} size={44} animate delay={0.4} />
          <Creature look={{ face: 'jellyfish' }} size={72} animate />
          <Creature look={{ face: 'penguin' }} size={44} animate delay={1.1} />
          <Creature look={{ face: 'shark' }} size={44} animate delay={1.8} />
        </div>
        {twoSteps && <p className="text-[13px] text-muted">Step {askName ? 1 : 2} of 2</p>}
        <h1 className="text-4xl font-normal tracking-[-0.02em]">
          {askName ? 'What should your Bots call you?' : 'Welcome to the reef'}
        </h1>
        <p className="text-base text-muted">
          {askName
            ? 'Your Bots use your name when they talk to you.'
            : 'Point Reef at a repo. Every Work item you start there keeps its own memory, and Bots work on it with you.'}
        </p>
        {askName ? <NameStep /> : <ProjectStep />}
        <p className="text-[13px] text-muted">
          Reef keeps its data in its own folder, never in your repo.
        </p>
      </div>
    </div>
  );
}
