import { expect, it } from 'vitest';
import { preview } from './preview';

it('keeps the words of a message on one line without its markdown', () => {
  expect(
    preview(
      '**Checkout form is ready.**\n\n- 3 files changed, +84 −12\n- See [PR #48](https://x.test/48) and `make test`\n\n## Next\n> done',
    ),
  ).toBe(
    'Checkout form is ready. · 3 files changed, +84 −12 · See PR #48 and make test · Next · done',
  );
});
