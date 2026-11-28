<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ConditionRow, { type ConditionKindOption, type ConditionValue } from './ConditionRow.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const kinds: ConditionKindOption[] = [
    { kind: 'title', label: 'Release title', input: 'pattern' },
    { kind: 'audio', label: 'Audio codec', input: 'values', values: [
      { value: 'aac', label: 'AAC' }, { value: 'ac3', label: 'AC3 (Dolby Digital)' }, { value: 'eac3', label: 'EAC3 (Dolby Digital Plus)' },
      { value: 'dts', label: 'DTS' }, { value: 'truehd', label: 'TrueHD' }] },
    { kind: 'language', label: 'Language', input: 'values', values: [{ value: 'en', label: 'English' }, { value: 'fr', label: 'French' }] },
    { kind: 'size', label: 'Size', input: 'range', unit: 'GB' },
  ];
  const audio: ConditionValue = { kind: 'audio', values: ['aac', 'ac3', 'eac3'] };

  const { Story } = defineMeta({ title: 'Organisms/ConditionRow', component: ConditionRow, tags: ['autodocs'],
    args: { value: audio, kinds, position: 1 } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Values, matching the test title"><ConditionRow value={audio} {kinds} position={1} matched onremove={() => {}} /></Row>
    <Row label="Pattern, negated and required, no match"><ConditionRow value={{ kind: 'title', pattern: '\\bDTS(-HD)?\\b', negate: true, required: true }} {kinds} position={2} matched={false} /></Row>
    <Row label="Language with except"><ConditionRow value={{ kind: 'language', values: ['en'], except: true }} {kinds} position={3} /></Row>
    <Row label="Size range"><ConditionRow value={{ kind: 'size', min: 1, max: 12 }} {kinds} position={4} /></Row>
    <Row label="Server error"><ConditionRow value={{ kind: 'title', pattern: '(dts' }} {kinds} position={5} errors={{ pattern: 'not a valid regular expression: missing closing )' }} /></Row>
    <Row label="Read-only (config file)"><ConditionRow value={audio} {kinds} position={6} readonly /></Row>
  </ThemePair>
</Story>
