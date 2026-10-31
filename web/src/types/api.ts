export type Project = {
  id: number;
  name: string;
  path: string;
  purpose: string;
  baseBranch: string;
  createdAt: string;
};

export type Profile = {
  name: string;
  occupation: string;
  style: string[];
  updatedAt: string;
};

export type MemoryItem = {
  id: number;
  kind: string;
  body: string;
  why: string;
  scope: string;
  author: string;
  botId: number | null;
  createdAt: string;
};

export type MemoryAdded = MemoryItem & {
  similar?: { id: number; body: string }[];
  embedError?: string;
};

export type WorkItemType =
  'feature' | 'bug' | 'incident' | 'investigation' | 'task' | 'improvement';

export type WorkItemStatus =
  'backlog' | 'queued' | 'in_progress' | 'needs_you' | 'review' | 'done' | 'dropped';

export type AcceptanceLine = {
  position: number;
  text: string;
  checked: boolean;
  note: string;
};

export type WorkItem = {
  id: number;
  projectId: number;
  parentId: number | null;
  type: WorkItemType;
  title: string;
  description: string;
  status: WorkItemStatus;
  teamId: number | null;
  proposedBy: string;
  suggestedTeamId: number | null;
  queuePosition: number | null;
  tokenBudget: number;
  createdAt: string;
  closedAt: string | null;
  acceptanceLines: AcceptanceLine[];
};

export type ProofCommand = { command: string; output: string };

export type Proof = {
  spaceId: number;
  acceptanceLines: AcceptanceLine[];
  commands: ProofCommand[];
  screenshots: string[];
  submittedAt: string;
};

export type Brief = {
  id: number;
  workItemId: number;
  body: string;
  author: string;
  createdAt: string;
};

export type Space = {
  id: number;
  kind: 'work_item' | 'free' | 'conversation';
  projectId: number;
  workItemId: number | null;
  name: string;
  team: string;
  role: string;
  botId: number | null;
  paused: string;
  createdAt: string;
  closedAt: string | null;
  movedTo: number | null;
};

export type SpaceDetail = Space & { tokenBudget: number; tokensUsed: number };

export type SpacePreview = Space & {
  members: number[];
  lastMessage: { author: string; body: string; createdAt: string } | null;
};

export type PinKind = 'bot' | 'chat' | 'space';

export type Pin = { kind: PinKind; id: number; title: string; createdAt: string };

export type FindResult = {
  kind: PinKind | 'workItem';
  id: number;
  title: string;
  projectId?: number;
};

export type ProposedItem = {
  kind: string;
  body: string;
  why?: string;
  scope: string;
  replaces?: number[];
};

export type Proposal = {
  id: number;
  spaceId: number;
  role: string;
  botId?: number | null;
  kind: 'wrap' | 'brief';
  brief: string;
  items: ProposedItem[];
  retire: number[];
  status: string;
  createdAt: string;
};

export type Message = {
  id: number;
  spaceId: number;
  parentId: number | null;
  author: string;
  botId?: number | null;
  body: string;
  createdAt: string;
};

export type Turn = {
  id: number;
  spaceId: number;
  role: string;
  botId?: number | null;
  status: string;
  error: string;
  wakeText: string;
  directMessageId?: number | null;
  retried: boolean;
  checkpoint: string | null;
  startedAt: string;
};

export type Role = {
  botId: number;
  name: string;
  lead: boolean;
  write: boolean;
  backend: string;
  provider?: string;
  permissionMode: string;
  description: string;
  skills: string[];
  face?: string;
  colour?: string;
  model?: string;
  tools: string[];
  mcp: { name: string; label: string }[];
};

export type Provider = {
  name: string;
  kind: string;
  detected: boolean;
  path?: string;
  version?: string;
  folder?: string;
  createdAt?: string;
};

export type SkillInfo = { name: string; description: string };

export type Skills = { claude: SkillInfo[]; codex: SkillInfo[] };

export type Bot = Omit<Role, 'lead' | 'botId'> & { id: number };

export type Team = {
  id: number;
  name: string;
  responsibility: string;
  roles: Role[];
};

export type DirectMessage = {
  id: number;
  spaceId: number;
  chainId: number;
  replyToId: number | null;
  sourceMessageId: number | null;
  from: string;
  fromBotId?: number | null;
  to: string;
  toBotId?: number | null;
  body: string;
  status: 'sent' | 'done' | 'failed';
  state?: 'waiting' | 'delivered' | 'answered' | 'failed' | 'stopped';
  createdAt: string;
};

export type InboxItem = {
  kind:
    | 'paused'
    | 'held'
    | 'failed'
    | 'review'
    | 'waiting'
    | 'sub_item'
    | 'proposed'
    | 'started'
    | 'space_proposed';
  group: 'needs_you' | 'review' | 'fyi';
  spaceId: number;
  workItemId: number | null;
  text: string;
  at: string;
  turnId?: number;
  messageId?: number;
  id?: number;
  proposal?: SpaceProposal;
};

export type SpaceProposal = {
  id: number;
  proposedBy: string;
  name: string;
  botIds: number[];
  coordinatorBotId: number;
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
  children?: InspectorToolCall[];
};

export type InspectorTurn = {
  id: number;
  role: string;
  botId: number | null;
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
  botId: number | null;
  turns: number;
  usage: Usage;
  costUsd: number | null;
};

export type Inspector = {
  roles: InspectorRole[];
  turns: InspectorTurn[];
};

export type WorkItemPR = {
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
