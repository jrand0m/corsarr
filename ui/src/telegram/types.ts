// Data contracts of the Telegram HTML widgets. Each widget renders a whole
// page body from one of these objects: no shell, drawer, store or event stream.
import type { MediaKind } from '../lib/tokens';
import type { Variant } from '../lib/components/organisms/VariantPicker/VariantPicker.svelte';
import type { QueueAction, QueueCardData } from '../lib/components/molecules/QueueCard/QueueCard.svelte';

/** The variant picker in Telegram: pick the release that replaces a download. */
export interface VariantPickerWidgetData {
  /** The download being replaced. */
  subject: {
    title: string;
    kind: MediaKind;
    subtitle?: string;
    /** "2160p WEB-DL, stalled at 81%". */
    current?: string;
  };
  title?: string;
  cap?: string;
  variants: Variant[];
  defaultId?: string;
  /** When the default is picked; the server picks it then even if the page never posts. */
  autoAcceptAt?: Date | string | number;
  /** Render time (ms). */
  now: number;
  /** POST target; receives `variant` and `action` (pick | cancel). */
  formAction: string;
}

/** Actions a Telegram queue card posts. Kill and both replaces are confirmed by the server. */
export type QueueWidgetAction = Extract<QueueAction, 'pause' | 'resume' | 'replace' | 'replace-lower' | 'kill'>;

/** The queue list in Telegram: the queue as compact cards with plain form actions. */
export interface QueueListWidgetData {
  title?: string;
  downloads: QueueCardData[];
  /** Render time (ms). */
  now: number;
  /** POST target; receives `id` and `action`. */
  formAction: string;
  /** Actions offered on every unfinished card; default all. */
  actions?: QueueWidgetAction[];
}
