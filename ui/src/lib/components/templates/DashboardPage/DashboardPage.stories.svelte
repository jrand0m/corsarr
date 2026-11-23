<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DashboardPage from './DashboardPage.svelte';
  import { dashboardAttention, dashboardTiles, dashboardHealth, dashboardHistory, downloads, NOW } from '../../../storybook/fixtures';

  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };
  const full = { attention: dashboardAttention, tiles: dashboardTiles, downloads: downloads.slice(0, 3), downloadsTotal: 5, totalSpeed: 12.4e6, health: dashboardHealth, history: dashboardHistory, now: NOW };

  const { Story } = defineMeta({ title: 'Templates/DashboardPage', component: DashboardPage, tags: ['autodocs'], args: full });
</script>

<Story name="Playground" globals={desktop} />

<Story name="Healthy and calm" asChild globals={desktop}>
  <DashboardPage healthySince="since 09:12" tiles={dashboardTiles} downloads={downloads.slice(0, 1)} totalSpeed={12.4e6} health={{ ok: 8, total: 8, problems: [] }} history={dashboardHistory} now={NOW} />
</Story>

<Story name="Something stuck, phone" asChild globals={phone}>
  <DashboardPage {...full} />
</Story>

<Story name="Something stuck, phone dark" asChild globals={{ ...phone, theme: 'dark' }}>
  <DashboardPage {...full} />
</Story>

<Story name="Loading" asChild globals={desktop}>
  <DashboardPage tiles={dashboardTiles} loading />
</Story>

<Story name="Partial failure" asChild globals={{ ...desktop, theme: 'dark' }}>
  <DashboardPage {...full} errors={{ health: 'The health service did not answer in 10 s.', history: 'HTTP 500' }} onretry={() => {}} />
</Story>
