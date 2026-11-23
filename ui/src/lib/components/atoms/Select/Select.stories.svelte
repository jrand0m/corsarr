<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Select from './Select.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const profiles = [
    { value: 'hd', label: 'HD 1080p' },
    { value: 'uhd', label: 'Living-room TV (2160p)' },
    { value: 'sd', label: 'Phone (720p)', disabled: true },
  ];
  const { Story } = defineMeta({
    title: 'Atoms/Select',
    component: Select,
    tags: ['autodocs'],
    args: { options: profiles, value: 'hd', 'aria-label': 'Quality profile' },
  });
</script>

<Story name="Playground" />

<Story name="All variants and states" asChild>
  <ThemePair>
    <Row label="Default, placeholder"><Select aria-label="Profile" options={profiles} value="uhd" /><Select aria-label="Root folder" options={profiles} placeholder="Choose a root folder" /></Row>
    <Row label="Sizes"><Select aria-label="Small" size="sm" options={profiles} value="hd" /><Select aria-label="Large" size="lg" options={profiles} value="hd" /></Row>
    <Row label="Invalid, disabled"><Select aria-label="Invalid" invalid options={profiles} placeholder="Required" /><Select aria-label="Disabled" disabled options={profiles} value="hd" /></Row>
  </ThemePair>
</Story>
