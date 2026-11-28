<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PluginSettingsForm, { type ActionReply } from './PluginSettingsForm.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import { harbourSchema, hostileSchema, unsupportedSchema } from '../../../storybook/plugin-module-fixtures';
  import { instanceMeta } from '../../../forms/plugin-schema';

  const saved = { url: 'http://10.0.0.20:8080', username: 'media', slots: '6', timeout: '1m', trackers: ['udp://tracker.example:1337'], paths: { '/downloads': '/data/downloads' } };
  const locks = instanceMeta(harbourSchema.fields, 'downloadClients', 'upstairs', { apikey: 'CORSARR_SECRET_DOWNLOADCLIENTS_UPSTAIRS_APIKEY' });
  const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
  async function onaction(id: string): Promise<ActionReply> {
    await wait(700);
    return id === 'test' ? { outcome: 'ok', text: 'Connected to Harbour 2.4 ✅' } : { outcome: 'warn', text: 'Found 2 categories.\nThe category "corsarr" is missing ⚠️' };
  }
  async function onsubmit(values: Record<string, unknown>) {
    await wait(400);
    return values.url ? [] : [{ path: 'url', message: 'is required' }];
  }

  const { Story } = defineMeta({
    title: 'Organisms/PluginSettingsForm', component: PluginSettingsForm, tags: ['autodocs'],
    args: { schema: harbourSchema, label: 'New Harbour instance', onaction, onsubmit },
  });
</script>

<Story name="Playground" />

<Story name="Draft with actions" asChild>
  <PluginSettingsForm schema={harbourSchema} label="New Harbour instance" {onaction} {onsubmit} submitLabel="Check and show YAML" />
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Configured instance: read-only with the managed lock">
      <PluginSettingsForm schema={harbourSchema} values={saved} meta={locks} label="Upstairs box settings" readonly {onaction} />
    </Row>
    <Row label="Server errors with the plugin hint">
      <PluginSettingsForm schema={harbourSchema} label="New Harbour instance" {onsubmit}
        problems={[{ path: 'url', message: 'must be an http or https address' }, { path: 'slots', message: 'must be at most 20' }, { path: 'extra', message: 'unknown setting' }]} />
    </Row>
    <Row label="Action running"><PluginSettingsForm schema={harbourSchema} values={saved} meta={locks} label="Running" readonly {onaction} result={{ action: 'Test connection', state: 'running' }} /></Row>
    <Row label="Action failed"><PluginSettingsForm schema={harbourSchema} values={saved} meta={locks} label="Failed" readonly {onaction} result={{ action: 'Test connection', state: 'fail', text: 'Harbour refused the API key 🔑' }} /></Row>
    <Row label="Action answer refused"><PluginSettingsForm schema={harbourSchema} values={saved} meta={locks} label="Refused" readonly {onaction} result={{ action: 'Find categories', state: 'refused' }} /></Row>
    <Row label="Settings cannot be shown"><PluginSettingsForm schema={unsupportedSchema} label="Broken plugin" /></Row>
    <Row label="Hostile text stays literal">
      <PluginSettingsForm schema={hostileSchema} label="Hostile plugin" {onaction} result={{ action: 'test', state: 'ok', text: '<img src=x onerror=alert(1)> https://evil.example' }} />
    </Row>
  </ThemePair>
</Story>
