export const claudeModels = [
  { value: '', label: 'Default' },
  { value: 'opus', label: 'Opus' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'haiku', label: 'Haiku' },
];

export function claudeModelOptions(model: string) {
  return claudeModels.some((o) => o.value === model)
    ? claudeModels
    : [...claudeModels, { value: model, label: model }];
}
