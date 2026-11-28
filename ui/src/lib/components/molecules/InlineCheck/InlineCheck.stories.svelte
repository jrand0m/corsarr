<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import InlineCheck from './InlineCheck.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import SecretField from '../SecretField/SecretField.svelte';
  import TextField from '../TextField/TextField.svelte';
  import UrlField from '../UrlField/UrlField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Molecules/InlineCheck', component: InlineCheck, tags: ['autodocs'],
    args: { family: 'sourceCheck', value: 'reachable', detail: 'Sonarr 4.0.20 · supported 3.x to 4.x' },
  });
</script>

<Story name="Playground" />

<Story name="Source check states" asChild>
  <ThemePair>
    <Row label="Checking"><InlineCheck family="sourceCheck" value="checking" /></Row>
    <Row label="Reachable"><InlineCheck family="sourceCheck" value="reachable" detail="Sonarr 4.0.20 · supported 3.x to 4.x" /></Row>
    <Row label="Address only"><InlineCheck family="sourceCheck" value="key-needed" detail="An app answers at this address. Enter its API key to confirm it is Sonarr and check its version." /></Row>
    <Row label="Unauthorised"><InlineCheck family="sourceCheck" value="unauthorised" detail="The app refused this API key." /></Row>
    <Row label="Wrong app"><InlineCheck family="sourceCheck" value="wrong-app" detail="This address is Radarr, not Sonarr." /></Row>
    <Row label="Unsupported version"><InlineCheck family="sourceCheck" value="unsupported" detail="Version 2.0.0.5344 is outside the supported range 3.x to 4.x." /></Row>
    <Row label="Unreachable"><InlineCheck family="sourceCheck" value="unreachable" detail="The host name sonarr does not resolve." /></Row>
  </ThemePair>
</Story>

<Story name="Path check states" asChild>
  <ThemePair>
    <Row label="Partly matched"><InlineCheck family="check" value="warn" detail="Matches 1 of 2 root folders." items={['Matches /data/media/tv', 'Unmapped /other/tv']} /></Row>
    <Row label="Read-only target"><InlineCheck family="check" value="pass" detail="Exists and readable. Read-only, which is fine for shadow mode." /></Row>
    <Row label="Missing target"><InlineCheck family="check" value="fail" detail="/srv/media does not exist for corsarr." /></Row>
    <Row label="Long sample"><InlineCheck family="check" value="warn" detail="1 of 8 root folders are missing for corsarr." maxItems={3}
      items={['/data/a → /media/a: found', '/data/b → /media/b: found', '/data/c → /media/c: missing', '/data/d', '/data/e', '/data/f', '/data/g', '/data/h']} /></Row>
    <Row label="Idle (renders nothing)"><InlineCheck family="check" /></Row>
  </ThemePair>
</Story>

<Story name="In fields with a Test action" asChild>
  <ThemePair>
    <Row label="Address">
      <UrlField label="Sonarr address" value="http://sonarr:8989">
        {#snippet action()}<Button size="sm" variant="secondary">Test</Button>{/snippet}
        {#snippet result()}<InlineCheck family="sourceCheck" value="key-needed" detail="An app answers at this address. Enter its API key to confirm it is Sonarr." />{/snippet}
      </UrlField>
    </Row>
    <Row label="API key">
      <SecretField label="Sonarr API key" value="0123456789abcdef">
        {#snippet action()}<Button size="sm" variant="secondary">Test</Button>{/snippet}
        {#snippet result()}<InlineCheck family="sourceCheck" value="reachable" detail="Sonarr 4.0.20 · supported 3.x to 4.x" />{/snippet}
      </SecretField>
    </Row>
    <Row label="Path prefix">
      <TextField label="Source path prefix" mono value="/data/media">
        {#snippet action()}<Button size="sm" variant="secondary">Test</Button>{/snippet}
        {#snippet result()}<InlineCheck family="check" value="pass" detail="Matches every root folder (2)." items={['Matches /data/media/tv', 'Matches /data/media/movies']} />{/snippet}
      </TextField>
    </Row>
  </ThemePair>
</Story>
