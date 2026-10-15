import { creatureName } from '@/components/creature';

const keywords: [string, string][] = [
  ['research', 'octopus'],
  ['secur', 'turtle'],
  ['design', 'seahorse'],
  ['writ', 'otter'],
  ['product', 'clownfish'],
  ['devops', 'manta-ray'],
];

export function suggestFace(roleName: string) {
  const name = roleName.trim().toLowerCase();
  return keywords.find(([word]) => name.includes(word))?.[1] ?? creatureName(name);
}
