// A loaded value with the states every screen shows: loading, error (with code and retry) and
// data. A newer load supersedes an older one, so a refetch after an event never paints stale rows.
import { untrack } from 'svelte';
import { ApiError, toApiError } from './api/client';

export class Resource<T> {
  data = $state<T | undefined>();
  loading = $state(true);
  error = $state<ApiError | undefined>();
  readonly #fn: (signal: AbortSignal) => Promise<T>;
  #ctl: AbortController | undefined;

  constructor(fn: (signal: AbortSignal) => Promise<T>) {
    this.#fn = fn;
  }

  /** Loads; `quiet` keeps the current data on screen (refetch after an event or reconnect). */
  async load(quiet = false): Promise<void> {
    this.#ctl?.abort();
    const ctl = new AbortController();
    this.#ctl = ctl;
    // Called from effects: reading state here must not subscribe the caller.
    untrack(() => { if (!quiet || this.data === undefined) this.loading = true; });
    try {
      const v = await this.#fn(ctl.signal);
      if (ctl.signal.aborted) return;
      this.data = v;
      this.error = undefined;
    } catch (e) {
      if (ctl.signal.aborted) return;
      this.error = toApiError(e);
    } finally {
      if (!ctl.signal.aborted) this.loading = false;
    }
  }

  /** Bound reload for ErrorState retry buttons. */
  retry = () => void this.load();

  abort(): void {
    this.#ctl?.abort();
  }
}
