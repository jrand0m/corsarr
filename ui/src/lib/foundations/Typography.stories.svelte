<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ThemePair from '../storybook/ThemePair.svelte';
  import { tokenGroups } from '../tokens';

  const { Story } = defineMeta({ title: 'Foundations/Typography', tags: ['autodocs'] });
  const roles = [
    ['text-2xl', 'Entity title', 'Harbour Lights', 'semibold'],
    ['text-xl', 'Page title', 'Queue', 'semibold'],
    ['text-lg', 'Section title', 'Active downloads', 'semibold'],
    ['text-base', 'Body, phone controls', 'Rejected: 2160p exceeds the Living-room TV profile.', 'regular'],
    ['text-md', 'Table cell, control, label', 'Harbour Lights · S02E05 · 1080p WEB', 'regular'],
    ['text-sm', 'Compact cell, badge', 'Import blocked', 'medium'],
    ['text-xs', 'Meta, caption', 'Last searched 5m ago', 'regular'],
    ['text-2xs', 'Kbd, bay legend', 'Ctrl K', 'medium'],
  ] as const;
</script>

<Story name="Scale and roles" asChild>
  <ThemePair>
    {#each roles as [token, role, sample, weight] (token)}
      <div style="display: grid; gap: var(--space-1)">
        <span style="font-size: var(--text-xs); color: var(--ink-muted)">{role} · --{token} · {weight}</span>
        <span style="font-size: var(--{token}); font-weight: var(--weight-{weight}); line-height: var(--leading-snug)">{sample}</span>
      </div>
    {/each}
  </ThemePair>
</Story>

<Story name="Numbers and mono" asChild>
  <ThemePair>
    <div style="display: grid; gap: var(--space-1); justify-items: end; width: max-content; font-size: var(--text-md)">
      <span class="num">1,111.1 MB</span><span class="num">988.8 MB</span><span class="num">40.4 GB</span>
    </div>
    <p style="font-size: var(--text-sm); color: var(--ink-muted)">Every number is tabular (tables, time, data, output, .num).</p>
    <code style="font-size: var(--text-sm)">/srv/media/tv/Harbour Lights/Season 02</code>
    <p style="font-size: var(--text-sm); color: var(--ink-muted)">Mono only for paths, hashes, keys, YAML and log lines.</p>
    <p style="font-size: var(--text-xs); color: var(--ink-subtle)">Tokens: {tokenGroups.text.join(', ')}</p>
  </ThemePair>
</Story>
