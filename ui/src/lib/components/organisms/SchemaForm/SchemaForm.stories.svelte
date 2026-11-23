<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SchemaForm from './SchemaForm.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { SetupDriver } from '../SetupFlow/SetupFlow.svelte';
  import type { FieldMeta } from '../../../forms/schema';
  import {
    everyFieldSchema, everyFieldValues, everyFieldMeta, categoryOptions, plexSchema, trackerSchema,
  } from '../../../storybook/plugin-schema';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const pause = (ms: number) => new Promise((r) => setTimeout(r, ms));
  const loadoptions = async () => { await pause(400); return categoryOptions; };
  const never = () => new Promise<never>(() => {});
  const failing = async () => { await pause(200); throw new Error('the client answered 401 Unauthorized'); };
  const onsave = async () => { await pause(600); };
  const refuse = async () => { await pause(400); return { url: 'Nothing answered at 10.0.0.20:8080 (connection refused)' }; };
  const driver: SetupDriver = {
    start: async () => { await pause(300); return { code: 'K7QP', url: 'https://plex.tv/link', expiresAt: now + 14 * 60_000 }; },
    wait: never,
    submit: async (_f, input) => { await pause(500); return input.password === 'wrong' ? { ok: false, error: 'The tracker refused those credentials.' } : { ok: true, message: 'Signed in. The session cookie is saved.' }; },
  };
  const managedMeta: Record<string, FieldMeta> = {
    ...everyFieldMeta,
    url: { managed: 'downloaders.harbourbox.url' },
    slots: { managed: 'downloaders.harbourbox.slots', drift: 'The client runs 5; the config file says 3. Apply the plan to fix it.' },
    priority: { drift: 'Changed on the client to High since the last sync.' },
  };
  const allManaged = Object.fromEntries(plexSchema.fields.map((f) => [f.key, { managed: `notifications.plex.${f.key}` }]));

  const { Story } = defineMeta({
    title: 'Organisms/SchemaForm',
    component: SchemaForm,
    tags: ['autodocs'],
    args: { schema: everyFieldSchema, values: everyFieldValues, meta: everyFieldMeta, label: 'Harbour box settings', loadoptions, onsave, now },
  });
</script>

<Story name="Playground" />

<Story name="Every field type" asChild>
  <ThemePair>
    <Row label="Saved values, dynamic options loading, conditional field hidden until Add downloads paused">
      <SchemaForm schema={everyFieldSchema} values={everyFieldValues} meta={everyFieldMeta} label="Harbour box settings" {loadoptions} {onsave} />
    </Row>
  </ThemePair>
</Story>

<Story name="New object, empty" asChild>
  <ThemePair>
    <Row label="Defaults filled; Save shows the inline errors and the summary">
      <SchemaForm schema={everyFieldSchema} label="New download client" {loadoptions} {onsave} saveLabel="Add client" />
    </Row>
  </ThemePair>
</Story>

<Story name="Server errors" asChild>
  <ThemePair>
    <Row label="Per-field messages from the server; editing the field clears its message">
      <SchemaForm schema={plexSchema} values={{ url: 'http://10.0.0.20:8080' }} meta={{ token: { isSet: true } }} errors={{ url: 'Nothing answered at 10.0.0.20:8080 (connection refused)' }} label="Plex settings" onsave={refuse} />
    </Row>
  </ThemePair>
</Story>

<Story name="Dynamic options" asChild>
  <ThemePair>
    <Row label="Loading"><SchemaForm schema={{ fields: [everyFieldSchema.fields[5]] }} label="Category loading" loadoptions={never} /></Row>
    <Row label="Failed, with reload"><SchemaForm schema={{ fields: [everyFieldSchema.fields[5]] }} label="Category failed" loadoptions={failing} /></Row>
  </ThemePair>
</Story>

<Story name="Managed and drift" asChild>
  <ThemePair>
    <Row label="Some fields locked by the config file, one drifted">
      <SchemaForm schema={everyFieldSchema} values={everyFieldValues} meta={managedMeta} label="Harbour box settings, partly managed" {loadoptions} {onsave} />
    </Row>
    <Row label="Every field managed: no Save">
      <SchemaForm schema={plexSchema} values={{ url: 'http://plex.lan:32400', refreshLibraries: true }} meta={allManaged} label="Plex settings, managed" />
    </Row>
  </ThemePair>
</Story>

<Story name="Setup flows" asChild>
  <ThemePair>
    <Row label="PIN pairing (Plex)"><SchemaForm schema={plexSchema} values={{ url: 'http://plex.lan:32400' }} label="Plex settings" setup={driver} {onsave} {now} /></Row>
    <Row label="Cookie paste and login form (type 'wrong' as the password to see the refusal)"><SchemaForm schema={trackerSchema} label="Tracker settings" setup={driver} {onsave} {now} /></Row>
    <Row label="Already paired"><SchemaForm schema={plexSchema} values={{ url: 'http://plex.lan:32400' }} meta={{ token: { isSet: true } }} label="Plex settings, paired" setup={driver} {onsave} {now} /></Row>
  </ThemePair>
</Story>

<Story name="Read-only" asChild>
  <ThemePair>
    <Row label="Viewing without edit rights"><SchemaForm schema={plexSchema} values={{ url: 'http://plex.lan:32400' }} meta={{ token: { isSet: true } }} label="Plex settings, read-only" readonly /></Row>
  </ThemePair>
</Story>
