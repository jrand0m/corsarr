<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CheckResults from './CheckResults.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { CheckResult } from './CheckResults.svelte';
  const now = new Date('2026-11-22T14:05:00Z');
  const checks: CheckResult[] = [
    { id: 'reach', name: 'Reach 10.0.0.20:8080', status: 'pass', ms: 38 },
    { id: 'auth', name: 'Sign in', status: 'pass', ms: 120 },
    { id: 'version', name: 'qBittorrent version', status: 'warn', detail: '4.3.9 works, but 4.5 or later keeps categories on restart.', ms: 12 },
    { id: 'path', name: 'Save path reachable from corsarr', status: 'fail', detail: '/downloads is not mounted in the corsarr container.', href: '#path-mapping', hrefLabel: 'Add a path mapping', ms: 4 },
    { id: 'cat', name: 'Category exists', status: 'skipped', detail: 'Needs a reachable save path.' },
  ];
  const running: CheckResult[] = checks.map((c, i) => ({ ...c, status: i < 2 ? 'pass' : i === 2 ? 'running' : 'pending', detail: undefined, href: undefined, ms: i < 2 ? c.ms : undefined }));
  const allPass: CheckResult[] = checks.slice(0, 3).map((c) => ({ ...c, status: 'pass', detail: undefined }));
  const { Story } = defineMeta({ title: 'Molecules/CheckResults', component: CheckResults, tags: ['autodocs'], args: { checks, title: 'Connection test', ranAt: '2026-11-22T14:03:00Z', now } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Failures first, with fix link"><CheckResults title="Connection test" {checks} ranAt="2026-11-22T14:03:00Z" {now} onrerun={() => {}} /></Row>
    <Row label="Running"><CheckResults title="Connection test" checks={running} onrerun={() => {}} /></Row>
    <Row label="All passed"><CheckResults title="Self-test" checks={allPass} ranAt="2026-11-22T13:00:00Z" {now} onrerun={() => {}} /></Row>
    <Row label="Not run yet"><CheckResults title="Conformance" checks={checks.map((c) => ({ id: c.id, name: c.name, status: 'pending' as const }))} onrerun={() => {}} /></Row>
  </ThemePair>
</Story>
