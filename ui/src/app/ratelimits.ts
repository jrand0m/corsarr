// Plain-words views of the learned rate limits shown in System › Health.
import type { RateCeiling, RateLimitStatus } from './api/types';

const units: [number, string][] = [[3600, 'hour'], [60, 'minute'], [1, 'second']];

/** "per second", "per minute", "per 10 seconds". */
export function perWindow(seconds: number): string {
  for (const [size, name] of units) {
    if (seconds === size) return `per ${name}`;
    if (seconds > size && seconds % size === 0) return `per ${seconds / size} ${name}s`;
  }
  return `per ${seconds} seconds`;
}

const trim = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(1));

export const sourceLabels: Record<RateCeiling['source'], string> = {
  advertised: 'announced by the service',
  declared: 'documented limit',
  throttle: 'found when throttled',
  probe: 'found by a check',
};

/** "10 per second (found when throttled)". */
export const ceilingText = (c: RateCeiling) => `${trim(c.limit)} ${perWindow(c.windowSeconds)} (${sourceLabels[c.source]})`;

/** The pace corsarr keeps: "8.5 a second", "12 a minute", "not paced". */
export function paceText(perSecond: number): string {
  if (perSecond <= 0) return 'Not paced';
  if (perSecond >= 1) return `${trim(perSecond)} a second`;
  if (perSecond * 60 >= 1) return `${trim(perSecond * 60)} a minute`;
  return `${trim(perSecond * 3600)} an hour`;
}

/** A host as it is; a plugin instance ("indexer/name") by its name and kind. */
export function serviceName(service: string): string {
  const [type, key] = service.split('/', 2);
  return key ? `${key} (${type})` : service;
}

/** Why a service is never re-checked, or undefined when it is. */
export function noCheckReason(r: RateLimitStatus): string | undefined {
  if (r.nextCheck) return undefined;
  if (r.ceilings.some((c) => c.source === 'advertised' || c.source === 'declared')) return 'Not needed: the limit is known';
  if (r.probe === 'off') return 'Never: this site is not checked';
  return 'Never';
}
