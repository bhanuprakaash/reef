import { expect, it } from 'vitest';
import { spaceRefLinks } from './space-ref-links';

it('turns two same-name references into links to their own ids', () => {
  expect(spaceRefLinks('#[Login](space:3) vs #[Login](space:9)')).toBe(
    '[#Login](/spaces/3) vs [#Login](/spaces/9)',
  );
});
