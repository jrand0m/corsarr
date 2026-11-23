<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ToggleMatrix, { type MatrixColumn, type MatrixRow, type MatrixValue } from './ToggleMatrix.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const events: MatrixRow[] = [
    { id: 'grab.sent', label: 'Release grabbed', group: 'Downloads' },
    { id: 'import.completed', label: 'Imported', group: 'Downloads' },
    { id: 'import.blocked', label: 'Import blocked', description: 'Needs a decision from you', group: 'Downloads' },
    { id: 'health.failing', label: 'Health check failing', group: 'System' },
    { id: 'update.available', label: 'Update available', group: 'System' },
  ];
  const connections: MatrixColumn[] = [
    { id: 'telegram', label: 'Telegram' },
    { id: 'email', label: 'Email' },
    { id: 'ntfy', label: 'ntfy' },
    { id: 'priority', label: 'Priority', kind: 'select', options: [
      { value: 'low', label: 'Low' }, { value: 'normal', label: 'Normal' }, { value: 'high', label: 'High' },
    ] },
  ];
  const value: MatrixValue = {
    'grab.sent': { telegram: true, priority: 'low' },
    'import.completed': { telegram: true, ntfy: true, priority: 'normal' },
    'import.blocked': { telegram: true, email: true, ntfy: true, priority: 'high' },
    'health.failing': { email: true, ntfy: true, priority: 'high' },
    'update.available': { email: true, priority: 'low' },
  };
  const unavailable = (r: MatrixRow, c: MatrixColumn) =>
    c.id === 'email' && r.id === 'grab.sent' ? 'Email is limited to one message an hour; grabs are too frequent' : undefined;

  const caps: MatrixRow[] = [
    { id: 'hevc', label: 'HEVC (x265)' }, { id: 'av1', label: 'AV1' }, { id: 'dv', label: 'Dolby Vision' }, { id: 'truehd', label: 'TrueHD Atmos' },
  ];
  const policy: MatrixColumn[] = [{ id: 'allow', label: 'Allow' }, { id: 'prefer', label: 'Prefer' }];

  const { Story } = defineMeta({
    title: 'Organisms/ToggleMatrix',
    component: ToggleMatrix,
    tags: ['autodocs'],
    args: { rows: events, columns: connections, value, label: 'Which connection hears which event', rowHeader: 'Event', unavailable },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Events × connections with priority, grouped, one cell unavailable">
      <div style="width: 100%"><ToggleMatrix rows={events} columns={connections} {value} label="Which connection hears which event" rowHeader="Event" {unavailable} /></div>
    </Row>
    <Row label="Capabilities × policy, empty">
      <div style="width: 100%"><ToggleMatrix rows={caps} columns={policy} label="Device capabilities" rowHeader="Capability" /></div>
    </Row>
    <Row label="Read-only (managed by config)">
      <div style="width: 100%"><ToggleMatrix rows={events.slice(0, 3)} columns={connections} {value} label="Events, managed" rowHeader="Event" readonly /></div>
    </Row>
  </ThemePair>
</Story>
