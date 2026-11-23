<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Countdown from './Countdown.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const at = (s: number) => new Date(Date.now() + s * 1000);
  const fixed = Date.parse('2026-11-22T14:05:00');
  const { Story } = defineMeta({ title: 'Atoms/Countdown', component: Countdown, tags: ['autodocs'] });
</script>

<Story name="Live" asChild>
  <ThemePair>
    <Row label="Ticks each second inside the last hour"><Countdown to={at(245)} action="Removes" tone="warn" /><Countdown to={at(59)} action="Retries" /></Row>
    <Row label="Longer spans"><Countdown to={at(3 * 3600 + 1200)} action="Next run" /><Countdown to={at(2 * 86400)} action="Airs" tone="info" /></Row>
  </ThemePair>
</Story>

<Story name="Fixed (for visual tests)" asChild>
  <ThemePair>
    <Row label="Tones"><Countdown to={fixed + 245_000} now={fixed} action="Removes" tone="warn" /><Countdown to={fixed + 30_000} now={fixed} action="Acts" tone="error" /><Countdown to={fixed + 7200_000} now={fixed} action="Next retry" /></Row>
    <Row label="Expired"><Countdown to={fixed - 1000} now={fixed} action="Removes" expiredLabel="acting now" tone="warn" /></Row>
  </ThemePair>
</Story>
