// Shared display clocks. One interval per resolution for the whole app, running only while
// something on screen reads it (createSubscriber). This is display refresh, never network polling.
import { createSubscriber } from 'svelte/reactivity';

function makeClock(intervalMs: number) {
  let now = Date.now();
  const subscribe = createSubscriber((update) => {
    now = Date.now();
    const id = setInterval(() => {
      now = Date.now();
      update();
    }, intervalMs);
    return () => clearInterval(id);
  });
  return {
    get now() {
      subscribe();
      return now;
    },
  };
}

/** Ticks every second: countdowns. */
export const secondClock = makeClock(1000);
/** Ticks every 30 s: relative timestamps. */
export const minuteClock = makeClock(30_000);

export function prefersReducedMotion(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
}
