<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Field from './Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Molecules/Field',
    component: Field,
    tags: ['autodocs'],
    args: { label: 'Host', help: 'Address of the qBittorrent Web UI as corsarr reaches it.', required: false },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}
    <Field {...args}>
      {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} invalid={c.invalid} readonly={c.readonly} required={c.required} disabled={c.disabled} value="10.0.0.20" />{/snippet}
    </Field>
  {/snippet}
</Story>


<Story name="All states" asChild>
  <ThemePair>
    <Row label="Default with help">
      <Field label="Category" help="Downloads land in this qBittorrent category.">
        {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} value="corsarr-tv" />{/snippet}
      </Field>
    </Row>
    <Row label="Required, empty">
      <Field label="Host" required>
        {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} required={c.required} placeholder="10.0.0.20" />{/snippet}
      </Field>
    </Row>
    <Row label="Error">
      <Field label="Port" error="Port must be between 1 and 65535; 80800 is too large." required>
        {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} invalid={c.invalid} value="80800" />{/snippet}
      </Field>
    </Row>
    <Row label="Managed by config">
      <Field label="API key header" managed="downloaders.qbit.headers.x-api-key">
        {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} readonly={c.readonly} value="X-Api-Key" />{/snippet}
      </Field>
    </Row>
    <Row label="Drift marker">
      <Field label="Seed ratio" drift="The config file says 2.0; the running value is 1.5 since a manual edit on 20 Nov.">
        {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} value="1.5" />{/snippet}
      </Field>
    </Row>
    <Row label="Disabled">
      <Field label="Tracker override" disabled help="Turn on private trackers to set this.">
        {#snippet children(c)}<TextInput id={c.id} aria-describedby={c.describedby} disabled={c.disabled} value="" />{/snippet}
      </Field>
    </Row>
  </ThemePair>
</Story>
