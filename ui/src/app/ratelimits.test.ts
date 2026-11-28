import { describe, expect, it } from 'vitest';
import { ceilingText, noCheckReason, paceText, perWindow, serviceName } from './ratelimits';
import type { RateLimitStatus } from './api/types';

const row = (o: Partial<RateLimitStatus> = {}): RateLimitStatus => ({
  service: 'api.example.org', ceilings: [], recent: [], pace: 0, probe: 'normal', probing: false, ...o,
});

describe('rate limit views', () => {
  it('names windows, ceilings and paces in plain words', () => {
    expect(perWindow(1)).toBe('per second');
    expect(perWindow(10)).toBe('per 10 seconds');
    expect(perWindow(120)).toBe('per 2 minutes');
    expect(perWindow(3600)).toBe('per hour');
    expect(perWindow(0.5)).toBe('per 0.5 seconds');
    expect(ceilingText({ windowSeconds: 60, limit: 40, source: 'advertised', foundAt: '' })).toBe('40 per minute (announced by the service)');
    expect(paceText(0)).toBe('Not paced');
    expect(paceText(8.5)).toBe('8.5 a second');
    expect(paceText(0.2)).toBe('12 a minute');
    expect(paceText(0.001)).toBe('3.6 an hour');
  });

  it('names services and why some are never checked', () => {
    expect(serviceName('indexer/usenet')).toBe('usenet (indexer)');
    expect(serviceName('api.example.org')).toBe('api.example.org');
    expect(noCheckReason(row({ nextCheck: '2026-12-01T00:00:00Z' }))).toBeUndefined();
    expect(noCheckReason(row({ ceilings: [{ windowSeconds: 1, limit: 5, source: 'declared', foundAt: '' }] }))).toMatch(/known/);
    expect(noCheckReason(row({ probe: 'off' }))).toMatch(/not checked/);
    expect(noCheckReason(row())).toBe('Never');
  });
});
