import type { FileDiff } from '@/types/api';

export type TreeNode =
  | { kind: 'dir'; path: string; name: string; children: TreeNode[] }
  | { kind: 'file'; name: string; file: FileDiff };

type Dir = Extract<TreeNode, { kind: 'dir' }>;

function compact(nodes: TreeNode[]): TreeNode[] {
  return nodes
    .map((node): TreeNode => {
      if (node.kind === 'file') return node;
      let dir = node;
      for (;;) {
        const [only] = dir.children;
        if (dir.children.length !== 1 || only.kind !== 'dir') break;
        dir = { ...only, name: `${dir.name}/${only.name}` };
      }
      return { ...dir, children: compact(dir.children) };
    })
    .sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === 'dir' ? -1 : 1));
}

export function buildFileTree(files: FileDiff[]): TreeNode[] {
  const root: TreeNode[] = [];
  for (const file of files) {
    const dirs = file.path.split('/');
    const name = dirs.pop() ?? file.path;
    let level = root;
    dirs.forEach((dirName, i) => {
      const path = dirs.slice(0, i + 1).join('/');
      let dir = level.find((n): n is Dir => n.kind === 'dir' && n.path === path);
      if (!dir) {
        dir = { kind: 'dir', path, name: dirName, children: [] };
        level.push(dir);
      }
      level = dir.children;
    });
    level.push({ kind: 'file', name, file });
  }
  return compact(root);
}
