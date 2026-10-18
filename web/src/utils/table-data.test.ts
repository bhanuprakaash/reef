import { describe, expect, it } from 'vitest';
import { csv, numericColumn, sortOrder } from '@/utils/table-data';

const column = (...cells: string[]) => cells.map((cell) => [cell]);

describe('sortOrder', () => {
  it('sorts plain numbers by value, negatives below zero', () => {
    const body = column('10', '-5', '$1,200.50', '−7', '0', '+3', '$-2', '45%');

    expect(sortOrder(body, 0, 'asc').map((i) => body[i][0])).toEqual([
      '−7',
      '-5',
      '$-2',
      '0',
      '+3',
      '10',
      '45%',
      '$1,200.50',
    ]);
    expect(sortOrder(body, 0, 'desc').map((i) => body[i][0])[0]).toBe('$1,200.50');
  });

  it('compares a column with any non-number cell as text, digit runs as numbers', () => {
    const body = column('500', '2k', '10 ms', 'item 10', 'item 9');

    expect(numericColumn(body, 0)).toBe(false);
    expect(sortOrder(body, 0, 'asc').map((i) => body[i][0])).toEqual([
      '2k',
      '10 ms',
      '500',
      'item 9',
      'item 10',
    ]);
  });

  it('needs every body cell to be a number', () => {
    expect(numericColumn(column('1', '-2.5', '3,000'), 0)).toBe(true);
    expect(numericColumn(column('1', ''), 0)).toBe(false);
    expect(numericColumn([], 0)).toBe(false);
  });
});

describe('csv', () => {
  it('quotes cells and guards formulas but not plain negative numbers', () => {
    expect(
      csv([
        ['name', 'n'],
        ['=cmd|"/c calc"!A1', '-5'],
        ['+1+1', '-3.2'],
        ['@sum(a1)', '-x'],
      ]),
    ).toBe(
      ['"name","n"', `"'=cmd|""/c calc""!A1","-5"`, `"'+1+1","-3.2"`, `"'@sum(a1)","'-x"`].join(
        '\n',
      ),
    );
  });
});
