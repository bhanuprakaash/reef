export type Project = {
  id: number;
  name: string;
  path: string;
  createdAt: string;
};

export type Feature = {
  id: number;
  projectId: number;
  slug: string;
  name: string;
  goal: string;
  stage: string;
  createdAt: string;
};

export type Brief = {
  id: number;
  featureId: number;
  body: string;
  author: string;
  createdAt: string;
};

export type Decision = {
  id: number;
  featureId: number;
  messageId: number | null;
  body: string;
  why: string;
  author: string;
  createdAt: string;
};

export type Channel = {
  id: number;
  projectId: number;
  featureId: number | null;
  name: string;
  team: string;
  role: string;
  backend: string;
  permissionMode: string;
  paused: string;
  createdAt: string;
};

export type ProposedDecision = {
  body: string;
  why: string;
};

export type Proposal = {
  id: number;
  channelId: number;
  role: string;
  brief: string;
  notes: string;
  decisions: ProposedDecision[];
  status: string;
  createdAt: string;
};

export type Message = {
  id: number;
  channelId: number;
  parentId: number | null;
  author: string;
  body: string;
  createdAt: string;
};

export type Team = {
  name: string;
  roles: { name: string; lead: boolean }[];
};

export type Approval = {
  id: number;
  channelId: number;
  from: string;
  to: string;
  messageId: number;
  text: string;
  status: string;
  createdAt: string;
};

export type TaskStatus = 'todo' | 'doing' | 'done';

export type Task = {
  id: number;
  projectId: number;
  featureId: number | null;
  title: string;
  body: string;
  status: TaskStatus;
  createdAt: string;
  updatedAt: string;
};
