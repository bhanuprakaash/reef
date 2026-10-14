export type Project = {
  id: number;
  name: string;
  path: string;
  createdAt: string;
};

export type ProjectMemory = {
  projectId: number;
  body: string;
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

export type Turn = {
  id: number;
  channelId: number;
  role: string;
  status: string;
  error: string;
  wakeText: string;
  retried: boolean;
  checkpoint: string | null;
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

export type InboxItem = {
  kind: 'approval' | 'paused' | 'failed' | 'waiting';
  channelId: number;
  featureId: number | null;
  text: string;
  at: string;
  approvalId?: number;
  turnId?: number;
};

export type DiffLineKind = 'add' | 'del' | 'ctx';

export type DiffLine = {
  kind: DiffLineKind;
  old?: number;
  new?: number;
  text: string;
};

export type DiffHunk = {
  header: string;
  lines: DiffLine[];
};

export type FileDiff = {
  path: string;
  status: 'added' | 'modified' | 'deleted';
  hunks: DiffHunk[];
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
