<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Drawer from './Drawer.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import ActionMenu from '../../molecules/ActionMenu/ActionMenu.svelte';
  import PropertyList, { type Property } from '../../molecules/PropertyList/PropertyList.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const props: Property[] = [
    { key: 'release', label: 'Release', value: 'The.Long.Haul.S02E03.1080p.WEB.h264-HARBOUR', mono: true, copy: true },
    { key: 'client', label: 'Downloader', value: 'qBittorrent · tv-corsarr' },
    { key: 'size', label: 'Size', value: '4.7 GB' },
    { key: 'added', label: 'Added', value: '22 Nov 2026, 13:41' },
    { key: 'hash', label: 'Info hash', value: '3f9a1c0e7b2d4a6f8e1b9c3d5a7f0e2b4c6d8a1f', mono: true, copy: true },
  ];
  const menu = [
    { id: 'recheck', label: 'Recheck', icon: 'refresh' as const },
    { id: 'kill', label: 'Kill download…', icon: 'trash' as const, danger: true, group: 'kill' },
  ];
  const { Story } = defineMeta({
    title: 'Organisms/Drawer',
    component: Drawer,
    tags: ['autodocs'],
    args: { title: 'The Long Haul S02E03 · Customs', subtitle: 'qBittorrent · tv-corsarr', size: 'md' },
  });
</script>

<script lang="ts">
  let open = $state(false);
</script>

{#snippet detail()}
  <div class="detail">
    <div class="badges"><StatusBadge family="download" value="stalled" detail="no peers for 2h" /></div>
    <PropertyList items={props} label="Download properties" />
  </div>
{/snippet}

{#snippet tools()}<ActionMenu items={menu} label="More actions for The Long Haul S02E03" />{/snippet}
{#snippet primary()}<Button>Pause</Button><Button variant="primary" icon="swap">Replace…</Button>{/snippet}

<Story name="Playground">
  {#snippet template(args)}<Drawer {...args} modal={false} open children={detail} actions={tools} footer={primary} />{/snippet}
</Story>

<Story name="Modal (slides in, Escape, return focus)" asChild>
  <Button onclick={() => (open = true)}>Open The Long Haul S02E03</Button>
  <Drawer bind:open title="The Long Haul S02E03 · Customs" subtitle="qBittorrent · tv-corsarr" children={detail} actions={tools} footer={primary} />
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="With header tools and footer"><Drawer modal={false} open title="The Long Haul S02E03 · Customs" subtitle="qBittorrent · tv-corsarr" children={detail} actions={tools} footer={primary} /></Row>
    <Row label="Small, body only"><Drawer modal={false} open size="sm" title="Dockside (2026)" children={detail} /></Row>
    <Row label="Long CJK title wraps">
      <Drawer modal={false} open size="sm" title="港の灯り — 東京湾岸コンテナターミナル・ドキュメンタリー" subtitle="SABnzbd · music">
        <p>Album · 9 tracks · FLAC</p>
      </Drawer>
    </Row>
  </ThemePair>
</Story>

<style>
  .detail { display: grid; gap: var(--space-4); }
  .badges { display: flex; gap: var(--space-1); }
</style>
