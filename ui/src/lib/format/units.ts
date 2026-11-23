// Unit split/join for the duration and size fields: a stored base value (seconds, bytes)
// is edited as an amount in the largest unit that represents it exactly.

export interface UnitDef<U extends string> { unit: U; factor: number; label: string }

export const durationUnits = [
  { unit: 's', factor: 1, label: 'seconds' },
  { unit: 'm', factor: 60, label: 'minutes' },
  { unit: 'h', factor: 3600, label: 'hours' },
  { unit: 'd', factor: 86400, label: 'days' },
] as const satisfies readonly UnitDef<string>[];
export type DurationUnit = (typeof durationUnits)[number]['unit'];

export const sizeUnits = [
  { unit: 'MB', factor: 1e6, label: 'MB' },
  { unit: 'GB', factor: 1e9, label: 'GB' },
  { unit: 'TB', factor: 1e12, label: 'TB' },
] as const satisfies readonly UnitDef<string>[];
export type SizeUnit = (typeof sizeUnits)[number]['unit'];

/** The largest allowed unit that divides `value` into a whole amount, else the smallest. */
export function splitUnit<U extends string>(
  value: number | null, units: readonly UnitDef<U>[], fallback: U,
): { amount: number | null; unit: U } {
  if (value === null || !Number.isFinite(value)) return { amount: null, unit: fallback };
  if (value === 0) return { amount: 0, unit: fallback };
  for (let i = units.length - 1; i >= 0; i--) {
    const u = units[i];
    if (Math.abs(value) >= u.factor && Number.isInteger(Math.round((value / u.factor) * 1000) / 1000)) {
      return { amount: Math.round((value / u.factor) * 1000) / 1000, unit: u.unit };
    }
  }
  const smallest = units[0];
  return { amount: Math.round((value / smallest.factor) * 1000) / 1000, unit: smallest.unit };
}

export function joinUnit<U extends string>(amount: number | null, unit: U, units: readonly UnitDef<U>[]): number | null {
  if (amount === null || !Number.isFinite(amount)) return null;
  const u = units.find((x) => x.unit === unit) ?? units[0];
  return Math.round(amount * u.factor);
}
