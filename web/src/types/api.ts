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
