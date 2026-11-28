<!--
  PermissionList: what a plugin module may touch, in words the owner approves: the hosts it may
  reach (exact names, any subdomain of a domain, or the host of one of its own settings) and the
  secret settings it may read. For an update it marks what the new version adds and lists what it
  drops. Marks are text plus an icon, never colour alone. Every name is shown as text.
-->
<script lang="ts" module>
  import type { ModulePermissions, PermissionDiff } from '../../../forms/plugin-types';

  const SETTING = '$setting:';

  /** One host pattern in words: "any subdomain of example.org", "the host in the Address setting". */
  export function hostText(h: string, labels: Record<string, string> = {}): string {
    if (h.startsWith(SETTING)) {
      const key = h.slice(SETTING.length);
      return `the host in the ${labels[key] ?? key} setting`;
    }
    if (h.startsWith('*.')) return `any subdomain of ${h.slice(2)}`;
    return h;
  }

  export type { ModulePermissions, PermissionDiff };
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';

  interface Props {
    permissions: ModulePermissions;
    /** Changes against the approved version; marks added entries and lists removed ones. */
    diff?: PermissionDiff;
    /** Setting labels by key, for host patterns bound to a setting. */
    labels?: Record<string, string>;
    /** Heading level of the two list titles. */
    level?: 3 | 4;
  }
  let { permissions, diff, labels = {}, level = 3 }: Props = $props();

  const hosts = $derived(permissions.http?.hosts ?? []);
  const secrets = $derived(permissions.secrets ?? []);
  const removedHosts = $derived(diff?.removedHosts ?? []);
  const removedSecrets = $derived(diff?.removedSecrets ?? []);
  const isNewHost = (h: string) => !!diff?.addedHosts.includes(h);
  const isNewSecret = (s: string) => !!diff?.addedSecrets.includes(s);
  const none = $derived(!hosts.length && !secrets.length && !removedHosts.length && !removedSecrets.length);
</script>

<div class="perms">
  {#if none}
    <p class="none"><Icon name="shield" size="sm" /><span>Needs no network access and reads no secrets.</span></p>
  {:else}
    <section class="part">
      <svelte:element this={`h${level}`} class="title">Network</svelte:element>
      {#if hosts.length || removedHosts.length}
        <ul>
          {#each hosts as h (h)}
            <li class:added={isNewHost(h)}>
              <Icon name="link" size="sm" />
              <span class="what">May contact <span class="name">{hostText(h, labels)}</span></span>
              {#if isNewHost(h)}<span class="mark new"><Icon name="plus" size="sm" />New</span>{/if}
            </li>
          {/each}
          {#each removedHosts as h (h)}
            <li class="removed">
              <Icon name="minus" size="sm" />
              <span class="what"><span class="name">{hostText(h, labels)}</span></span>
              <span class="mark gone">No longer needed</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="empty">No network access.</p>
      {/if}
    </section>
    <section class="part">
      <svelte:element this={`h${level}`} class="title">Secrets</svelte:element>
      {#if secrets.length || removedSecrets.length}
        <ul>
          {#each secrets as s (s)}
            <li class:added={isNewSecret(s)}>
              <Icon name="key" size="sm" />
              <span class="what">May read the <span class="name">{labels[s] ?? s}</span> secret</span>
              {#if isNewSecret(s)}<span class="mark new"><Icon name="plus" size="sm" />New</span>{/if}
            </li>
          {/each}
          {#each removedSecrets as s (s)}
            <li class="removed">
              <Icon name="minus" size="sm" />
              <span class="what"><span class="name">{labels[s] ?? s}</span></span>
              <span class="mark gone">No longer needed</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="empty">Reads no secrets.</p>
      {/if}
    </section>
  {/if}
</div>

<style>
  .perms { display: grid; gap: var(--space-4); min-width: 0; }
  .part { display: grid; gap: var(--space-2); min-width: 0; }
  .title { font-size: var(--text-md); font-weight: var(--weight-semibold); }
  ul { list-style: none; padding: 0; margin: 0; display: grid; gap: var(--space-1-5); }
  li { display: flex; align-items: center; gap: var(--space-2); font-size: var(--text-md); color: var(--ink); min-width: 0; flex-wrap: wrap; }
  li > :global(svg) { color: var(--ink-muted); flex: none; }
  .what { min-width: 0; overflow-wrap: anywhere; }
  .name { font-family: var(--font-mono); font-size: var(--text-sm); }
  .removed .what { color: var(--ink-muted); text-decoration: line-through; }
  .mark { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-xs); font-weight: var(--weight-medium); padding: 0 var(--space-1-5); border-radius: var(--radius-sm); }
  .new { color: var(--tone-warn-ink); background: var(--tone-warn-tint); border: var(--border-w) solid var(--tone-warn-line); }
  .gone { color: var(--ink-muted); }
  .none, .empty { display: flex; align-items: center; gap: var(--space-2); color: var(--ink-muted); font-size: var(--text-md); }
</style>
