<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MediaKindIcon from './MediaKindIcon.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import { mediaKinds } from '../../../tokens';

  const { Story } = defineMeta({ title: 'Atoms/MediaKindIcon', component: MediaKindIcon, tags: ['autodocs'], args: { kind: 'tv', showLabel: true } });
</script>

<Story name="Playground" />

<Story name="All kinds" asChild>
  <ThemePair>
    <Row label="Icon only (labelled for assistive tech)">{#each mediaKinds as kind (kind)}<MediaKindIcon {kind} />{/each}</Row>
    <Row label="With label">{#each mediaKinds as kind (kind)}<MediaKindIcon {kind} showLabel />{/each}</Row>
    <Row label="Large">{#each mediaKinds as kind (kind)}<MediaKindIcon {kind} size="lg" />{/each}</Row>
  </ThemePair>
</Story>
