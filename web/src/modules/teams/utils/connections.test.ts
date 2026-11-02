import { expect, it } from 'vitest';
import { pairsText, parsePairs } from './connections';

it('shows header values masked and sends the mask back unchanged', () => {
  const text = pairsText({ Authorization: '****', 'X-Team': '****' }, ': ');

  expect(text).toBe('Authorization: ****\nX-Team: ****');
  expect(parsePairs(text, ': ')).toEqual({ Authorization: '****', 'X-Team': '****' });
});

it('keeps a colon inside a header value', () => {
  expect(parsePairs('Referer: https://a.example:8080/x', ': ')).toEqual({
    Referer: 'https://a.example:8080/x',
  });
});

it('parses environment lines on the first =', () => {
  expect(parsePairs('TOKEN=a=b\nbad line\nHOME=/x')).toEqual({ TOKEN: 'a=b', HOME: '/x' });
});
