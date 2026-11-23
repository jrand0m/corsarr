import { describe, it, expect } from 'vitest';
import { fuzzyMatch, highlight } from './fuzzy';

describe('fuzzyMatch', () => {
  it('matches characters in order, case-insensitively', () => {
    expect(fuzzyMatch('lh', 'The Long Haul')!.indices).toEqual([4, 9]);
    expect(fuzzyMatch('HAUL', 'The Long Haul')).not.toBeNull();
    expect(fuzzyMatch('xyz', 'The Long Haul')).toBeNull();
    expect(fuzzyMatch('lu', 'Haul')).toBeNull();
  });

  it('ranks prefixes, word starts and contiguous runs higher', () => {
    const s = (q: string, t: string) => fuzzyMatch(q, t)!.score;
    expect(s('queue', 'Queue')).toBeGreaterThan(s('queue', 'Requeue stalled'));
    expect(s('dock', 'Dockside')).toBeGreaterThan(s('dock', 'Harbour docks'));
    expect(s('gq', 'Go to Queue')).toBeGreaterThan(s('gq', 'Toggle quality'));
  });

  it('treats an empty query as a match for everything', () => {
    expect(fuzzyMatch('  ', 'Anything')).toEqual({ score: 0, indices: [] });
  });
});

describe('highlight', () => {
  it('splits text into matched runs', () => {
    expect(highlight('Queue', [0, 1])).toEqual([{ text: 'Qu', hit: true }, { text: 'eue', hit: false }]);
  });
});
