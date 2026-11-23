<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ThemePair from '../storybook/ThemePair.svelte';
  import Row from '../storybook/Row.svelte';
  import Swatch from './Swatch.svelte';
  import StatusBadge from '../components/atoms/StatusBadge/StatusBadge.svelte';
  import { tokenGroups, tones } from '../tokens';

  const { Story } = defineMeta({ title: 'Foundations/Colours', tags: ['autodocs'] });
</script>

<Story name="Semantic roles" asChild>
  <ThemePair>
    <Row label="Surfaces (galvanised steel)">{#each tokenGroups.surface as t (t)}<Swatch token={t} />{/each}</Row>
    <Row label="Ink on raised surface">{#each tokenGroups.ink.slice(0, 4) as t (t)}<Swatch token="surface-raised" ink={t} />{/each}</Row>
    <Row label="Lines">{#each tokenGroups.line as t (t)}<Swatch token={t} />{/each}</Row>
    <Row label="Accent (container cobalt)">{#each tokenGroups.accent as t (t)}<Swatch token={t} />{/each}<Swatch token="accent" ink="ink-on-accent" /></Row>
    <Row label="Danger"><Swatch token="danger" ink="ink-on-danger" /><Swatch token="danger-hover" ink="ink-on-danger" /></Row>
    <Row label="Shell (Corten oxide)">{#each tokenGroups.shell.filter((t) => !t.includes('ink')) as t (t)}<Swatch token={t} ink="shell-ink" />{/each}</Row>
    <Row label="Media kinds (always with an icon or label)">{#each tokenGroups.kind as t (t)}<Swatch token={t} ink="ink-on-accent" />{/each}</Row>
  </ThemePair>
</Story>

<Story name="State tones" asChild>
  <ThemePair>
    {#each tones as tone (tone)}
      <Row label={tone}>
        <Swatch token="tone-{tone}-tint" ink="tone-{tone}-ink" />
        <Swatch token="tone-{tone}-line" />
        <Swatch token="tone-{tone}-solid" />
        <StatusBadge {tone} label={tone} icon="info" />
      </Row>
    {/each}
  </ThemePair>
</Story>
