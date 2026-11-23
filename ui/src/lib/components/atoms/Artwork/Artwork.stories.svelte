<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Artwork from './Artwork.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  // Synthetic placeholder art (inline SVG, no remote assets).
  const poster = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 3"><rect width="2" height="3" fill="#2f5d73"/><circle cx="1.4" cy=".8" r=".35" fill="#e9c46a"/><path d="M0 2.2 .8 1.6 1.4 2 2 1.7V3H0z" fill="#1b3a4b"/></svg>')}`;
  const { Story } = defineMeta({ title: 'Atoms/Artwork', component: Artwork, tags: ['autodocs'], args: { title: 'Harbour Lights', kind: 'tv', src: poster, width: 96 } });
</script>

<Story name="Playground" />

<Story name="Shapes and fallbacks" asChild>
  <ThemePair>
    <Row label="Loaded"><Artwork title="Harbour Lights" kind="tv" src={poster} /><Artwork title="Harbour Lights" kind="tv" src={poster} width={48} /></Row>
    <Row label="Missing art: initials + kind">
      <Artwork title="The Long Meridian" kind="movie" />
      <Artwork title="Salt and Signal" kind="music" shape="cover" />
      <Artwork title="Ledger of Tides" kind="book" />
      <Artwork title="北の灯台" kind="tv" />
    </Row>
    <Row label="Broken URL falls back"><Artwork title="Broken Image" kind="movie" src="/does-not-exist.jpg" /></Row>
    <Row label="Small sizes, avatar"><Artwork title="Night Ferry" kind="tv" width={32} /><Artwork title="Owner" kind="music" shape="avatar" width={40} /></Row>
  </ThemePair>
</Story>
