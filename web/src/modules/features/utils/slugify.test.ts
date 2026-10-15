import { describe, expect, it } from 'vitest';
import { isValidSlug, slugify } from './slugify';

describe('slugify', () => {
  it.each([
    ['Polish 1', 'polish-1'],
    ['  Hello  World!! ', 'hello-world'],
    ['UI/UX', 'ui-ux'],
    ['!!!', ''],
  ])('%j becomes %j', (name, slug) => {
    expect(slugify(name)).toBe(slug);
  });
});

describe('isValidSlug', () => {
  it.each([
    ['abc', true],
    ['abc-', true],
    ['2024-', true],
    ['2024', false],
    ['-a', false],
    ['Bad Slug', false],
    ['', false],
  ])('%j is %s', (slug, valid) => {
    expect(isValidSlug(slug)).toBe(valid);
  });
});
