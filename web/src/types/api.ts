export type Project = {
  id: number;
  name: string;
  path: string;
  baseBranch: string;
  createdAt: string;
};

export type Profile = {
  name: string;
  occupation: string;
  style: string[];
  updatedAt: string;
};

export type ProjectMemory = {
  projectId: number;
  body: string;
};

export type FeatureKind = 'journey' | 'fixes';

export type Feature = {
  id: number;
  projectId: number;
  slug: string;
  name: string;
  goal: string;
  stage: string;
  kind: FeatureKind;
  createdAt: string;
  closedAt: string | null;
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
  closedAt: string | null;
};

export type ChannelPreview = Channel & {
  lastMessage: { author: string; body: string; createdAt: string } | null;
};

export type ProposedDecision = {
  body: string;
  why: string;
};

export type Proposal = {
  id: number;
  channelId: number;
  role: string;
  kind: 'wrap' | 'brief';
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
  startedAt: string;
};

export type Role = {
  name: string;
  lead: boolean;
  write: boolean;
  backend: string;
  permissionMode: string;
  persona: string;
  skills: string[];
  face?: string;
  model?: string;
  tools: string[];
  mcp: { name: string; label: string }[];
  mcpRemove?: { name: string; label: string }[];
};

export type SkillInfo = { name: string; description: string };

export type Skills = { claude: SkillInfo[]; codex: SkillInfo[] };

export type Gate = { from: string; to: string; on: boolean };

export type Team = {
  name: string;
  roles: Role[];
  gates?: Gate[];
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
  messageId?: number;
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

export type Usage = {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
};

export type InspectorToolCall = {
  name: string;
  input: string;
  result: string;
};

export type InspectorTurn = {
  id: number;
  role: string;
  startedAt: string;
  status: string;
  model: string;
  usage: Usage;
  costUsd: number | null;
  prompt: string;
  toolCalls: InspectorToolCall[];
};

export type InspectorRole = {
  role: string;
  turns: number;
  usage: Usage;
  costUsd: number | null;
};

export type Inspector = {
  roles: InspectorRole[];
  turns: InspectorTurn[];
};

export type FeaturePR = {
  number: number;
  title: string;
  state: 'OPEN' | 'MERGED' | 'CLOSED';
  url: string;
  isDraft: boolean;
  reviewDecision: string;
  base: string;
  head: string;
  canMerge: boolean;
  checks: { name: string; state: 'pass' | 'fail' | 'pending' }[];
  body: string;
  additions: number;
  deletions: number;
  files: { path: string; additions: number; deletions: number }[];
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
