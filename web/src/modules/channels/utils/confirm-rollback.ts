export function confirmRollback(turnId: number) {
  return window.confirm(
    `Roll back to before turn ${turnId}? The files and the branch go back to that point, and later commits are dropped. The agent's session is not rewound.`,
  );
}
