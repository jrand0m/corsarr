<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PermissionList from './PermissionList.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const labels = { url: 'Address', apiKey: 'API key', password: 'Password' };
  const full = { http: { hosts: ['$setting:url', 'api.harbour.example', '*.cdn.example'] }, secrets: ['apiKey'] };
  const grown = { http: { hosts: ['$setting:url', 'api.harbour.example', 'metrics.harbour.example'] }, secrets: ['apiKey', 'password'] };
  const diff = { addedHosts: ['metrics.harbour.example'], removedHosts: ['*.cdn.example'], addedSecrets: ['password'], removedSecrets: [] };

  const { Story } = defineMeta({ title: 'Molecules/PermissionList', component: PermissionList, tags: ['autodocs'], args: { permissions: full, labels } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Hosts, a setting-bound host and a secret"><PermissionList permissions={full} {labels} /></Row>
    <Row label="Only the host of a setting"><PermissionList permissions={{ http: { hosts: ['$setting:url'] } }} {labels} /></Row>
    <Row label="Secrets only"><PermissionList permissions={{ http: {}, secrets: ['apiKey'] }} {labels} /></Row>
    <Row label="Nothing"><PermissionList permissions={{ http: {} }} /></Row>
    <Row label="Update: added and removed marks"><PermissionList permissions={grown} {diff} {labels} /></Row>
    <Row label="Hostile names stay text"><PermissionList permissions={{ http: { hosts: ['<img src=x onerror=alert(1)>.example'] }, secrets: ['<b>key</b>'] }} /></Row>
  </ThemePair>
</Story>
