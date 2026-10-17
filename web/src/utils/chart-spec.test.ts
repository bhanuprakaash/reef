import { describe, expect, it } from 'vitest';
import { parseChart } from './chart-spec';

describe('parseChart', () => {
  it('reads a valid spec and drops unknown keys', () => {
    const spec = parseChart(
      '{"type":"bar","title":"Builds","labels":["Mon","Tue"],"datasets":[{"label":"ok","data":[3,5],"color":"red"}],"x":1}',
    );

    expect(spec).toEqual({
      type: 'bar',
      title: 'Builds',
      labels: ['Mon', 'Tue'],
      datasets: [{ label: 'ok', data: [3, 5] }],
    });
  });

  it('accepts a spec with no title or dataset label', () => {
    expect(parseChart('{"type":"pie","labels":["a"],"datasets":[{"data":[1]}]}')).toEqual({
      type: 'pie',
      title: undefined,
      labels: ['a'],
      datasets: [{ label: undefined, data: [1] }],
    });
  });

  it.each([
    ['half-written json', '{"type":"bar","labels":["a"'],
    ['empty', ''],
    ['not an object', '[1,2]'],
    ['null', 'null'],
    ['unknown type', '{"type":"radar","labels":["a"],"datasets":[{"data":[1]}]}'],
    ['title not a string', '{"type":"bar","title":3,"labels":["a"],"datasets":[{"data":[1]}]}'],
    ['no labels', '{"type":"bar","labels":[],"datasets":[{"data":[]}]}'],
    ['label not a string', '{"type":"bar","labels":[1],"datasets":[{"data":[1]}]}'],
    ['no datasets', '{"type":"bar","labels":["a"],"datasets":[]}'],
    ['dataset not an object', '{"type":"bar","labels":["a"],"datasets":[[1]]}'],
    ['data length differs', '{"type":"bar","labels":["a","b"],"datasets":[{"data":[1]}]}'],
    ['data not numbers', '{"type":"bar","labels":["a"],"datasets":[{"data":["1"]}]}'],
  ])('returns null for %s', (_, text) => {
    expect(parseChart(text)).toBeNull();
  });
});
