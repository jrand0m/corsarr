<!--
  AvailabilityLine: whether the whole payload can be fetched, in words, plus the evidence the
  indexer gave (file count, times downloaded, the seed ratio the tracker asks for). Missing
  evidence is left out, never shown as 0.
-->
<script lang="ts" module>
  export type AvailabilityStatus = 'complete' | 'may_be_incomplete' | 'no_peers' | 'unknown';
  export type Protocol = 'torrent' | 'usenet' | 'source';

  /** The plain sentence for a status on a protocol. */
  export function availabilityText(status: AvailabilityStatus, protocol: Protocol): string {
    switch (status) {
      case 'complete': return protocol === 'source' ? 'The source serves whole files' : 'At least one full copy is seeded';
      case 'may_be_incomplete': return 'No one is seeding a full copy';
      case 'no_peers': return 'Nobody is sharing this torrent';
      default: return protocol === 'torrent' ? 'The indexer did not report seeders' : 'The indexer does not report completeness';
    }
  }
</script>

<script lang="ts">
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    status: AvailabilityStatus;
    protocol: Protocol;
    grabs?: number;
    files?: number;
    minimumRatio?: number;
  }
  let { status, protocol, grabs, files, minimumRatio }: Props = $props();

  const evidence = $derived([
    files ? `${formatCount(files)} ${files === 1 ? 'file' : 'files'}` : '',
    grabs ? `downloaded ${formatCount(grabs)} ${grabs === 1 ? 'time' : 'times'}` : '',
    minimumRatio ? `seed to ratio ${minimumRatio.toFixed(1)}` : '',
  ].filter(Boolean));
</script>

<span class="avail">
  <StatusBadge family="availability" value={status} size="sm" appearance="plain" />
  <span class="why">{availabilityText(status, protocol)}</span>
  {#if evidence.length}<span class="evidence">{evidence.join(' · ')}</span>{/if}
</span>

<style>
  .avail { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--space-0-5) var(--space-2); font-size: var(--text-sm); color: var(--ink-muted); }
  .evidence { color: var(--ink-subtle); }
</style>
