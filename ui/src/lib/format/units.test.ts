import { describe, it, expect } from 'vitest';
import { splitUnit, joinUnit, durationUnits, sizeUnits } from './units';

describe('splitUnit / joinUnit', () => {
  it('picks the largest exact unit', () => {
    expect(splitUnit(5400, durationUnits, 'm')).toEqual({ amount: 90, unit: 'm' });
    expect(splitUnit(7200, durationUnits, 'm')).toEqual({ amount: 2, unit: 'h' });
    expect(splitUnit(172800, durationUnits, 'm')).toEqual({ amount: 2, unit: 'd' });
    expect(splitUnit(45, durationUnits, 'm')).toEqual({ amount: 45, unit: 's' });
    expect(splitUnit(1.5e9, sizeUnits, 'GB')).toEqual({ amount: 1500, unit: 'MB' });
    expect(splitUnit(4e12, sizeUnits, 'GB')).toEqual({ amount: 4, unit: 'TB' });
  });

  it('keeps empty and zero in the fallback unit', () => {
    expect(splitUnit(null, durationUnits, 'm')).toEqual({ amount: null, unit: 'm' });
    expect(splitUnit(0, sizeUnits, 'GB')).toEqual({ amount: 0, unit: 'GB' });
  });

  it('joins back to the base value', () => {
    expect(joinUnit(90, 'm', durationUnits)).toBe(5400);
    expect(joinUnit(2.5, 'GB', sizeUnits)).toBe(2.5e9);
    expect(joinUnit(null, 'h', durationUnits)).toBeNull();
  });
});
