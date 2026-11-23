// The toast queue behind ToastRegion. Screens push outcomes here after an action
// ("Removed … and blocklisted the release", with undo); the region shows, stacks, announces
// and dismisses them. One queue per app, created by the shell and passed down (no global).

export interface ToastInput {
  message: string;
  detail?: string;
  tone?: 'ok' | 'info' | 'warn' | 'error';
  onundo?: () => void;
  undoLabel?: string;
  actionLabel?: string;
  onaction?: () => void;
  /** Milliseconds; at least 10 s with undo (enforced by Toast); 0 keeps it until dismissed. */
  duration?: number;
}

export interface ToastEntry extends ToastInput {
  id: number;
}

export class ToastQueue {
  items = $state<ToastEntry[]>([]);
  #next = 1;

  /** Adds a toast; returns its id for `dismiss`. */
  push(toast: ToastInput): number {
    const id = this.#next++;
    this.items.push({ ...toast, id });
    return id;
  }

  dismiss(id: number): void {
    this.items = this.items.filter((t) => t.id !== id);
  }

  clear(): void {
    this.items = [];
  }
}
