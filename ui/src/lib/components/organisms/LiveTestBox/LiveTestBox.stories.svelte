<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LiveTestBox, { type TestResult } from './LiveTestBox.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  // A toy reader for synthetic titles; the real one is the server's parser.
  const pick = (re: RegExp, s: string) => re.exec(s)?.[1] ?? null;
  const parse = (s: string): TestResult => {
    const fields = [
      { label: 'Title', value: pick(/^(.+?)[. ](?:S\d|\d{4})/i, s)?.replace(/\./g, ' ') ?? null },
      { label: 'Season and episode', value: pick(/(S\d{2}E\d{2})/i, s), mono: true },
      { label: 'Year', value: pick(/[. ](\d{4})[. ]/, s) },
      { label: 'Resolution', value: pick(/(2160p|1080p|720p|480p)/i, s) },
      { label: 'Source', value: pick(/(WEB-DL|WEBRip|BluRay|HDTV)/i, s) },
      { label: 'Release group', value: pick(/-(\w+)$/, s), mono: true },
    ];
    const known = fields.filter((f) => f.value).length;
    return { verdict: { matched: known >= 4, text: known >= 4 ? 'Read with confidence' : 'Partly read' }, fields };
  };
  const conditions = (s: string): TestResult => {
    const conds = [
      { label: 'Resolution is 2160p', matched: /2160p/i.test(s), required: true, detail: /2160p/i.test(s) ? "matched '2160p'" : "no '2160p' token", score: 0 },
      { label: 'HDR present', matched: /\b(HDR|DV|HDR10)\b/i.test(s), score: 50, detail: /\bHDR10?\b/i.test(s) ? "matched 'HDR'" : undefined },
      { label: 'Group on the trusted list', matched: /-(HARBOUR|DOCKSIDE)$/i.test(s), score: 25 },
      { label: 'Not a re-encode', matched: !/x265.*-(YTS|RARBG)/i.test(s), score: 0 },
    ];
    const req = conds.filter((c) => c.required).every((c) => c.matched);
    const score = conds.filter((c) => c.matched).reduce((n, c) => n + (c.score ?? 0), 0);
    return { verdict: { matched: req, text: req ? 'Format applies' : 'Format does not apply' }, score, conditions: conds };
  };
  const slow = async (s: string) => { await new Promise((r) => setTimeout(r, 900)); return parse(s); };
  const broken = async () => { throw new Error('the parser is restarting (HTTP 503)'); };
  const samples = ['The.Long.Haul.S02E03.1080p.WEB-DL.DDP5.1-HARBOUR', 'Quay.Lights.2024.2160p.BluRay.HDR10.x265-DOCKSIDE', 'night shift at the docks'];

  const { Story } = defineMeta({
    title: 'Organisms/LiveTestBox',
    component: LiveTestBox,
    tags: ['autodocs'],
    args: { label: 'Release title', test: parse, samples, placeholder: 'Paste a release title' },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Empty"><div style="width: 100%"><LiveTestBox label="Release title" test={parse} {samples} placeholder="Paste a release title" /></div></Row>
    <Row label="Fields known and unknown (parse tester)"><div style="width: 100%"><LiveTestBox label="Release title" test={parse} value={samples[0]} /></div></Row>
    <Row label="Partly read"><div style="width: 100%"><LiveTestBox label="Release title" test={parse} value={samples[2]} /></div></Row>
    <Row label="Per-condition match with score (custom format)"><div style="width: 100%"><LiveTestBox label="Test this format against" test={conditions} value={samples[1]} /></div></Row>
    <Row label="Format does not apply"><div style="width: 100%"><LiveTestBox label="Test this format against" test={conditions} value={samples[0]} /></div></Row>
    <Row label="Testing (slow answer)"><div style="width: 100%"><LiveTestBox label="Release title" test={slow} value={samples[0]} /></div></Row>
    <Row label="Error"><div style="width: 100%"><LiveTestBox label="Release title" test={broken} value={samples[0]} /></div></Row>
    <Row label="Manual (Test button)"><div style="width: 100%"><LiveTestBox label="Release title" test={parse} live={false} value={samples[0]} /></div></Row>
  </ThemePair>
</Story>
