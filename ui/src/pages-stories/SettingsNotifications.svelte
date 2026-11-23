<!--
  Page composition (story only): Settings › Connections › one notification connection.
  Per-service tabs build shoutrrr URLs from typed forms; the Raw URL tab takes any URL; both
  converge on the same config. The event matrix decides what is sent where and how loudly.
-->
<script lang="ts">
  import Shell from './Shell.svelte';
  import SettingsPage from '../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import CodeViewer from '../lib/components/molecules/CodeViewer/CodeViewer.svelte';
  import TextField from '../lib/components/molecules/TextField/TextField.svelte';
  import Tabs from '../lib/components/molecules/Tabs/Tabs.svelte';
  import SchemaForm from '../lib/components/organisms/SchemaForm/SchemaForm.svelte';
  import ToggleMatrix from '../lib/components/organisms/ToggleMatrix/ToggleMatrix.svelte';
  import {
    NOW, notifyColumns, notifyEvents, notifyMatrix, ntfySchema, ntfyValues, pushoverMeta, pushoverSchema, settingsSections,
    telegramMeta, telegramSchema, telegramValues,
  } from '../lib/storybook/fixtures';

  let { tab = 'telegram' }: { tab?: string } = $props();

  // svelte-ignore state_referenced_locally
  let current = $state(tab);
  let raw = $state('telegram://123456:AAH-dockside-alerts-token@telegram?chats=@dockside_alerts&preview=no');
  // Mask the credential part (between "://" and "@") so the preview never shows a secret.
  const masked = $derived(raw.replace(/^([a-z+]+:\/\/)([^@/]+)@/i, (_, scheme) => `${scheme}••••@`));
  const tabs = [
    { id: 'telegram', label: 'Telegram' }, { id: 'ntfy', label: 'ntfy' }, { id: 'pushover', label: 'Pushover' }, { id: 'raw', label: 'Raw URL' },
  ];
  const unavailable = (r: { id: string }, c: { id: string }) =>
    c.id === 'pushover' && r.id === 'grab.sent' ? 'Pushover is kept for problems; grabs are too frequent for its message limit' : undefined;
</script>

<Shell path="/settings">
  <SettingsPage sections={settingsSections} current="connections" title="Phone and chat" description="Notification connection · sends through 3 services">
    {#snippet actions()}<Button icon="bolt">Test all</Button>{/snippet}

    <section class="block" aria-labelledby="sn-services">
      <h2 id="sn-services">Services</h2>
      <p class="help">Each tab builds one service address. Use Raw URL for a service without its own tab.</p>
      <div class="panel">
        <Tabs {tabs} bind:value={current} label="Services">
          {#snippet children(id)}
            <div class="tab">
              {#if id === 'telegram'}
                <SchemaForm schema={telegramSchema} values={telegramValues} meta={telegramMeta} label="Telegram" saveLabel="Save Telegram" onsave={() => {}} now={NOW} />
              {:else if id === 'ntfy'}
                <SchemaForm schema={ntfySchema} values={ntfyValues} label="ntfy" saveLabel="Save ntfy" onsave={() => {}} now={NOW} />
              {:else if id === 'pushover'}
                <SchemaForm schema={pushoverSchema} values={{ devices: [] }} meta={pushoverMeta} label="Pushover" saveLabel="Save Pushover" onsave={() => {}} now={NOW} />
              {:else}
                <div class="raw">
                  <TextField label="Service URL" bind:value={raw} mono help="Any shoutrrr URL, such as ntfy://ntfy.example/topic or pushover://shoutrrr:TOKEN@USER/. Stored as a secret: never logged or exported in clear." />
                  <CodeViewer code={masked} label="Masked preview" filename="Masked preview" lineNumbers={false} download={false} wrap />
                  <div class="raw-actions"><Button icon="bolt">Test this URL</Button><Button variant="primary">Save URL</Button></div>
                </div>
              {/if}
            </div>
          {/snippet}
        </Tabs>
      </div>
    </section>

    <section class="block" aria-labelledby="sn-events">
      <h2 id="sn-events">Events</h2>
      <p class="help">What each service hears about, and the priority it is sent with.</p>
      <ToggleMatrix rows={notifyEvents} columns={notifyColumns} value={notifyMatrix} label="Events per service" rowHeader="Event" {unavailable} />
    </section>
  </SettingsPage>
</Shell>

<style>
  .block { display: grid; gap: var(--space-3); min-width: 0; }
  h2 { font-size: var(--text-lg); margin-top: var(--space-2); }
  .help { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  .panel { padding: var(--space-2) var(--space-5) var(--space-5); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); min-width: 0; }
  .tab { padding-top: var(--space-4); min-width: 0; }
  .raw { display: grid; gap: var(--space-4); min-width: 0; }
  .raw-actions { display: flex; gap: var(--space-2); justify-content: flex-end; flex-wrap: wrap; }
  @media (width < 48rem) {
    .panel { padding: var(--space-1) var(--space-3) var(--space-4); }
  }
</style>
