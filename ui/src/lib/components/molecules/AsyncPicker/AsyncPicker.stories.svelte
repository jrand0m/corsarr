<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AsyncPicker from './AsyncPicker.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { PickerOption, PickerSearch } from './AsyncPicker.svelte';
  const profiles: PickerOption[] = [
    { value: 'hd', label: 'HD 1080p', description: 'WEB and Blu-ray up to 1080p, cutoff WEB-DL 1080p' },
    { value: 'uhd', label: 'Ultra HD', description: '2160p with HDR, cutoff Remux 2160p' },
    { value: 'sd', label: 'SD', description: 'DVD and SDTV' },
    { value: 'any', label: 'Any', description: 'Grab the first acceptable release' },
    { value: 'legacy', label: 'Legacy 720p', description: 'Kept for old series', disabled: true },
  ];
  const series: PickerOption[] = [
    { value: '1', label: 'The Long Haul', description: '2024 · 2 seasons', kind: 'tv' },
    { value: '2', label: 'Dockside', description: '2026', kind: 'movie' },
    { value: '3', label: 'Harbour Lights', description: 'Album · 2019', kind: 'music' },
  ];
  const tags: PickerOption[] = ['4k', 'kids', 'anime', 'hdr', 'archive'].map((t) => ({ value: t, label: t }));
  const from = (list: PickerOption[], ms = 300): PickerSearch => (q, signal) =>
    new Promise((resolve, reject) => {
      const t = setTimeout(() => resolve(list.filter((o) => o.label.toLowerCase().includes(q.toLowerCase()))), ms);
      signal.addEventListener('abort', () => { clearTimeout(t); reject(new DOMException('aborted', 'AbortError')); });
    });
  const search = from(profiles);
  const failing: PickerSearch = () => Promise.reject(new Error('HTTP 503 from /api/v1/profiles'));
  const { Story } = defineMeta({ title: 'Molecules/AsyncPicker', component: AsyncPicker, tags: ['autodocs'], args: { search, label: 'Quality profile', noun: 'profiles', placeholder: 'Search profiles' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Single, picked"><AsyncPicker label="Quality profile" noun="profiles" search={from(profiles)} value={[profiles[0]]} /></Row>
    <Row label="Items with kind icons, 2 characters to search"><AsyncPicker label="Series" noun="items" search={from(series)} minChars={2} placeholder="Search the library" /></Row>
    <Row label="Multiple (tags)"><AsyncPicker label="Tags" noun="tags" search={from(tags, 150)} multiple value={[tags[0], tags[3]]} placeholder="Add a tag" /></Row>
    <Row label="Search fails (focus to see)"><AsyncPicker label="Quality profile" noun="profiles" search={failing} placeholder="Search profiles" /></Row>
    <Row label="Invalid"><AsyncPicker label="Quality profile" noun="profiles" search={from(profiles)} invalid placeholder="Required" /></Row>
    <Row label="Read-only (managed)"><AsyncPicker label="Tags" noun="tags" search={from(tags)} multiple value={[tags[1]]} readonly /></Row>
    <Row label="Disabled"><AsyncPicker label="Quality profile" noun="profiles" search={from(profiles)} disabled placeholder="Choose a kind first" /></Row>
  </ThemePair>
</Story>
