import { describe, it, expect } from 'vitest';
import { formatBytes, formatSpeed, formatGbPerHour, formatDuration, formatClock, formatCount, formatPercent } from './quantity';
import { formatRelative } from './time';

describe('quantity formatters', () => {
  it('formats bytes in SI units', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1530)).toBe('1.5 KB');
    expect(formatBytes(4_700_000_000)).toBe('4.7 GB');
    expect(formatBytes(123_400_000)).toBe('123 MB');
    expect(formatBytes(NaN)).toBe('—');
  });
  it('formats speed, GB/h, counts and percent', () => {
    expect(formatSpeed(2_400_000)).toBe('2.4 MB/s');
    expect(formatGbPerHour(3e9, 3600 * 2)).toBe('1.5 GB/h');
    expect(formatGbPerHour(3e9, 0)).toBe('—');
    expect(formatCount(12, 13)).toBe('12 of 13');
    expect(formatCount(5000)).toBe('5,000');
    expect(formatPercent(0.425)).toBe('43%');
    expect(formatPercent(2)).toBe('100%');
  });
  it('formats durations compactly and as clocks', () => {
    expect(formatDuration(45)).toBe('45s');
    expect(formatDuration(12 * 60 + 5)).toBe('12m');
    expect(formatDuration(3 * 3600 + 20 * 60)).toBe('3h 20m');
    expect(formatDuration(2 * 86400 + 4 * 3600)).toBe('2d 4h');
    expect(formatClock(245)).toBe('4:05');
    expect(formatClock(3729)).toBe('1:02:09');
    expect(formatClock(-3)).toBe('0:00');
  });
});

describe('formatRelative', () => {
  const now = new Date('2026-11-22T12:00:00Z');
  it('describes past and future', () => {
    expect(formatRelative(new Date('2026-11-22T11:59:50Z'), now)).toBe('just now');
    expect(formatRelative(new Date('2026-11-22T11:55:00Z'), now)).toBe('5m ago');
    expect(formatRelative(new Date('2026-11-22T15:00:00Z'), now)).toBe('in 3h');
    expect(formatRelative(new Date('2026-11-21T12:00:00Z'), now)).toBe('yesterday');
  });
});
