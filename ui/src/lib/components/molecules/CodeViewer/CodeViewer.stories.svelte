<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CodeViewer from './CodeViewer.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  const yaml = `# corsarr configuration (export of 22 Nov 2026)
downloaders:
  qbit:
    url: http://10.0.0.20:8080
    category: corsarr-tv   # one category per kind
    password: \${file:/run/secrets/qbit_password}
indexers:
  nzbgeek:
    api_key: \${env:NZBGEEK_API_KEY}
    categories: [5030, 5040, 5045]
rules:
  stalled:
    mode: dry-run
    grace: 90m
`;
  const env = `# Required variables
CORSARR_INDEXERS_NZBGEEK_API_KEY=
export CORSARR_DOWNLOADERS_QBIT_URL=http://10.0.0.20:8080
CORSARR_RULES_STALLED_MODE=dry-run`;
  const json = `{
  "event": "import.completed",
  "item": { "kind": "tv", "title": "The Long Haul", "episode": "S02E03" },
  "release": "The.Long.Haul.S02E03.1080p.WEB.h264-HARBOUR",
  "path": "/data/media/tv/The Long Haul/Season 02/The Long Haul - S02E03 - Customs [WEBDL-1080p].mkv"
}`;
  const { Story } = defineMeta({ title: 'Molecules/CodeViewer', component: CodeViewer, tags: ['autodocs'], args: { code: yaml, language: 'yaml', filename: 'corsarr.yaml' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="YAML"><CodeViewer code={yaml} language="yaml" filename="corsarr.yaml" /></Row>
    <Row label="env"><CodeViewer code={env} language="env" filename="corsarr.env" /></Row>
    <Row label="JSON webhook sample, wrapped, no download"><CodeViewer code={json} language="json" filename="Webhook sample" wrap download={false} /></Row>
    <Row label="Rendered message, no line numbers"><CodeViewer code={'Imported: The Long Haul S02E03 "Customs"\n1080p WEB · 2.1 GB · from NZBgeek'} language="text" filename="Telegram message" lineNumbers={false} download={false} /></Row>
  </ThemePair>
</Story>
