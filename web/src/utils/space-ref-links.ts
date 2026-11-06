export function spaceRefLinks(body: string) {
  return body.replace(/#\[([^\]\n]*)\]\(space:(\d+)\)/g, '[#$1](/spaces/$2)');
}
