<!--
  System › Migration: connect existing Sonarr, Radarr, Lidarr and Prowlarr installations (read-only), choose what
  comes across, review the plan (every source object with its outcome and reason; leave objects out),
  then apply it. Imported kinds start in shadow mode and every automation rule in dry-run; the kind
  modes beside the steps switch a kind to active when the owner is ready. Past runs keep their reports.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import ManagedValue, { managedFrom } from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Timestamp from '../../../lib/components/atoms/Timestamp/Timestamp.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import BoolField from '../../../lib/components/molecules/BoolField/BoolField.svelte';
  import CodeViewer from '../../../lib/components/molecules/CodeViewer/CodeViewer.svelte';
  import InlineCheck from '../../../lib/components/molecules/InlineCheck/InlineCheck.svelte';
  import MultiselectField from '../../../lib/components/molecules/MultiselectField/MultiselectField.svelte';
  import NumberField from '../../../lib/components/molecules/NumberField/NumberField.svelte';
  import SecretField from '../../../lib/components/molecules/SecretField/SecretField.svelte';
  import SegmentedControl from '../../../lib/components/molecules/SegmentedControl/SegmentedControl.svelte';
  import SourceProgressStrip from '../../../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
  import StatTile from '../../../lib/components/molecules/StatTile/StatTile.svelte';
  import TextField from '../../../lib/components/molecules/TextField/TextField.svelte';
  import UrlField from '../../../lib/components/molecules/UrlField/UrlField.svelte';
  import DataTable, { type Column } from '../../../lib/components/organisms/DataTable/DataTable.svelte';
  import FilterBar from '../../../lib/components/organisms/FilterBar/FilterBar.svelte';
  import WizardPage from '../../../lib/components/templates/WizardPage/WizardPage.svelte';
  import { emptyFilter, type FilterState } from '../../../lib/filters/url';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { toApiError } from '../../api/client';
  import {
    applyCount, categories, categoryLabel, configNotice, countTiles, defaultForm, filterEntries, fromPrefixView, hasConfigNotice, InstantCheck, outcomeView,
    pathCheckBody, pathCheckReady, pathCheckSig, planFacets, progressOf, requestBody, sampleView, scopeProblems, sourceApps, sourceCheckBody,
    sourceCheckReady, sourceCheckSig, sourceCheckView, sourceProblems, toggleExclude, toPrefixView, writes, type CheckView, type KindState,
    type MigrationReport, type MigrationRun, type PathCheck, type PathCheckInput, type PlanEntry, type SourceApp, type SourceCheck, type SourceCheckInput,
    type SourceForm, type StreamLine,
  } from '../../migration';

  const { api, live, toasts, status } = app();
  const steps = [
    { id: 'sources', label: 'Sources', description: 'corsarr only reads from them; nothing there changes.' },
    { id: 'scope', label: 'What comes across', description: 'Categories, path mapping and how the imported kinds start.' },
    { id: 'plan', label: 'Review the plan', description: 'Every source object with what would happen to it and why. Nothing is written yet.' },
    { id: 'apply', label: 'Apply', description: 'Writes the reviewed plan. Running it again later only picks up changes.' },
  ];

  let form = $state(defaultForm());
  let current = $state('sources');
  let completed = $state<string[]>([]);
  let lines = $state<StreamLine[]>([]);
  let running = $state<'plan' | 'apply' | undefined>();
  let plan = $state<MigrationReport | undefined>();
  let result = $state<MigrationReport | undefined>();
  let runError = $state<string | undefined>();
  let filter = $state<FilterState>(emptyFilter());

  const modes = new Resource((s) => api.list<KindState>('migration/modes', undefined, s));
  const runs = new Resource((s) => api.list<MigrationRun>('migration/runs', undefined, s));

  // Instant checks: each source is tested as its address and key are entered, and the path
  // rewrite as its prefixes are; a change cancels the check in flight.
  let sourceViews = $state<Partial<Record<SourceApp, CheckView<SourceCheck>>>>({});
  let pathView = $state<CheckView<PathCheck>>({ status: 'idle' });
  const sourceChecks = Object.fromEntries(sourceApps.map(({ app: a }) => [a, new InstantCheck<SourceCheckInput, SourceCheck>({
    run: (i, signal) => api.request<SourceCheck>('POST', 'migration/check/source', { body: sourceCheckBody(i), signal }),
    sig: sourceCheckSig, ready: sourceCheckReady, onview: (v) => (sourceViews[a] = v),
  })])) as Record<SourceApp, InstantCheck<SourceCheckInput, SourceCheck>>;
  const pathCheck = new InstantCheck<PathCheckInput, PathCheck>({
    run: (i, signal) => api.request<PathCheck>('POST', 'migration/check/paths', { body: pathCheckBody(i), signal }),
    sig: pathCheckSig, ready: pathCheckReady, onview: (v) => (pathView = v),
  });
  const full = (s: SourceForm): SourceCheckInput => ({ app: s.app, url: s.url, apiKey: s.apiKey, withKey: true });
  const pathInput = (): PathCheckInput => ({ sources: $state.snapshot(form.sources), fromPrefix: form.fromPrefix, toPrefix: form.toPrefix });

  // Feed each check only when its own input changed, so editing one source leaves the others alone.
  const seen = new Map<string, string>();
  $effect(() => {
    for (const s of form.sources) {
      const sig = s.include ? sourceCheckSig(full(s)) : '';
      if (seen.get(s.app) === sig) continue;
      seen.set(s.app, sig);
      if (s.include) sourceChecks[s.app].input(full(s));
      else { sourceChecks[s.app].cancel(); sourceViews[s.app] = { status: 'idle' }; }
    }
    const p = pathInput();
    const psig = pathCheckSig(p);
    if (seen.get('paths') !== psig) {
      seen.set('paths', psig);
      pathCheck.input(p);
    }
  });

  onMount(() => {
    void modes.load();
    void runs.load();
    if (!status.data) void status.load();
    const off = refetchOn(live, ['migration.run_completed', 'mediakind.mode_changed'], () => { void modes.load(true); void runs.load(true); });
    return () => {
      off();
      for (const c of Object.values(sourceChecks)) c.cancel();
      pathCheck.cancel();
    };
  });

  const notice = $derived(configNotice(form, modes.data ?? []));
  const pathResult = $derived(pathView.status === 'done' ? pathView.result : undefined);

  const chosen = $derived(form.sources.filter((s) => s.include).map((s) => s.app));
  const shown = $derived(plan ? filterEntries(result?.entries ?? plan.entries ?? [], filter) : []);
  const facets = $derived(planFacets(result?.entries ?? plan?.entries ?? []));
  const report = $derived(result ?? plan);

  async function stream(mode: 'plan' | 'apply') {
    running = mode;
    lines = [];
    runError = undefined;
    try {
      for await (const l of api.postStream<StreamLine>(`migration/${mode}`, requestBody(form, mode === 'apply' ? plan?.planHash : undefined))) {
        lines = [...lines, l];
        if ('error' in l) runError = l.error.message;
        if ('summary' in l) settle(mode, l.summary);
      }
    } catch (e) {
      runError = toApiError(e).message;
    } finally {
      running = undefined;
    }
  }

  function settle(mode: 'plan' | 'apply', s: { report: MigrationReport; planChanged?: boolean }) {
    if (mode === 'plan' || s.planChanged) {
      plan = s.report;
      result = undefined;
      if (s.planChanged) {
        runError = 'The sources or corsarr changed since you reviewed the plan. Review it again, then apply.';
        current = 'plan';
      }
      return;
    }
    result = s.report;
    toasts.push({ message: 'Migration applied', tone: 'ok' });
  }

  function validate(step: string): string[] {
    if (step === 'sources') return sourceProblems(form);
    if (step === 'scope') return scopeProblems(form);
    if (step === 'plan') return running ? ['Wait for the plan to finish.'] : plan?.entries?.length ? [] : ['There is no plan to apply yet.'];
    return [];
  }

  function onstep(step: string) {
    if (step === 'scope') pathCheck.blur(pathInput());
    if (step === 'plan') void stream('plan');
  }

  function finish() {
    if (result) {
      form = defaultForm();
      plan = result = undefined;
      completed = [];
      current = 'sources';
      return;
    }
    void stream('apply');
  }

  async function setMode(kind: string, mode: string) {
    try {
      await api.put(`migration/modes/${kind}`, { mode });
      toasts.push({ message: `${kindName(kind)}: ${mode} mode`, tone: 'ok' });
    } catch (e) {
      toasts.push({ message: `${kindName(kind)} mode unchanged`, detail: toApiError(e).message, tone: 'error' });
    }
    void modes.load(true);
  }

  const excluded = (e: PlanEntry) => form.exclude.includes(e.id);
  const columns: Column<PlanEntry>[] = [
    { id: 'outcome', label: 'Outcome', width: '10.5rem', value: (e) => outcomeView(e.outcome).label, phone: 'trailing' },
    { id: 'name', label: 'Object', value: (e) => e.name, phone: 'title' },
  ];
  const sourceName = (app: string) => sourceApps.find((s) => s.app === app)?.label ?? app;
  const kindName = (k: string) => ({ tv: 'TV', movie: 'Movies', music: 'Music', book: 'Books' } as Record<string, string>)[k] ?? k;
  const modeName = (m: string) => m.charAt(0).toUpperCase() + m.slice(1);
  const listed = (items: string[]) => items.length < 2 ? items.join('') : `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`;
</script>

{#snippet cell(e: PlanEntry, c: Column<PlanEntry>)}
  {#if c.id === 'outcome'}
    {@const v = outcomeView(e.outcome)}
    {#if !result && excluded(e)}<StatusBadge family="migration" value="skip" label="Left out" size="sm" />
    {:else}<StatusBadge family={v.family} value={v.value} label={v.label} size="sm" />{/if}
  {:else if c.id === 'name'}<span class="obj"><span class="oname">{e.name}</span><span class="why">{e.reason}</span></span>
  {:else}{c.value?.(e)}{/if}
{/snippet}

{#snippet pathTest(what: string)}
  <Button size="sm" variant="secondary" disabled={!pathCheckReady(pathInput())} onclick={() => pathCheck.test(pathInput())}>
    Test<span class="visually-hidden"> {what}</span>
  </Button>
{/snippet}

{#snippet pathLine(view: (c: PathCheck) => { value: string; detail: string; items?: string[] })}
  {#if pathView.status === 'checking'}<InlineCheck family="check" value="running" label="Checking…" />
  {:else if pathView.status === 'error'}<InlineCheck family="check" value="fail" detail="corsarr could not run the check: {pathView.error}" />
  {:else if pathResult}{@const l = view(pathResult)}<InlineCheck family="check" value={l.value} detail={l.detail} items={l.items} />{/if}
{/snippet}

{#snippet configBanner()}
  {#if notice.nothingWritten}
    <Banner tone="warn" icon="alert-triangle" title="This run imports nothing">
      Every category you chose only becomes a config snippet. Nothing reaches the library until you add the snippet to the config.
      Choose Library, Profiles, Blocklist, History or In-flight downloads to import something.
    </Banner>
  {:else if hasConfigNotice(notice)}
    <Banner preset="managed" title="Some of this is set by the config">
      <ul class="notice">
        {#each notice.kinds as k (k.kind)}
          <li>{k.label} mode is set by the config and stays as it is. To start it in {form.activate ? 'active' : 'shadow'} mode, put this in the config:
            <CodeViewer code={k.snippet} language="yaml" maxHeight="6rem" /></li>
        {/each}
        {#if notice.snippetOnly.length}<li>{listed(notice.snippetOnly)} only become a config snippet. Nothing changes until you add it to the config.</li>{/if}
        {#if notice.importing.length}<li>{listed(notice.importing)} import as usual.</li>{/if}
      </ul>
    </Banner>
  {/if}
{/snippet}

{#snippet table()}
  {#if report}
    <div class="tiles">
      {#each countTiles(report.counts) as t (t.outcome)}<StatTile label={t.label} value={t.n} />{/each}
    </div>
    <FilterBar {facets} bind:value={filter} searchLabel="Filter the plan" placeholder="Filter by name or reason" shortcut={false} />
    <DataTable rows={shown} {columns} {cell} rowKey={(e) => e.id} label="Migration plan" noun="object" height="28rem"
      rowActions={(e) => (result || !writes(e) ? [] : [{ id: 'toggle', label: excluded(e) ? 'Include again' : 'Leave out', icon: excluded(e) ? 'undo' : 'minus' }])}
      onaction={(_, e) => (form.exclude = toggleExclude(form.exclude, e.id))}>
      {#snippet expand(e: PlanEntry)}
        <div class="detail">
          <p>{e.reason}</p>
          <p class="note">{categoryLabel(e.category)} · {e.source ? sourceName(e.source) : 'corsarr'} · {e.id}{e.target ? ` → ${e.target}` : ''}</p>
        </div>
      {/snippet}
    </DataTable>
    {#if report.config}
      <p class="note">These objects live in the config file. Add the snippet to it and set {report.env?.join(', ')} in the environment; secrets are never copied.</p>
      <CodeViewer code={report.config} language="yaml" filename="migrated.yaml" download maxHeight="18rem" />
    {/if}
  {/if}
{/snippet}

<WizardPage title="Migration" description="Bring an existing Sonarr, Radarr, Lidarr or Prowlarr installation into corsarr without adding everything again."
  {steps} bind:current bind:completed {validate} {onstep} busy={!!running} finishLabel={result ? 'Start another run' : 'Apply plan'}
  onfinish={finish}>
  {#snippet children(step)}
    {#if runError}<Banner tone="error" icon="alert-triangle" title="The run stopped" live>{runError}</Banner>{/if}
    {#if step.id === 'sources'}
      <div class="sources">
        {#each form.sources as s, i (s.app)}
          {@const meta = sourceApps[i]}
          <fieldset>
            <legend class="visually-hidden">{meta.label}</legend>
            <BoolField label="Read from {meta.label}" help={meta.note} bind:checked={s.include} />
            {#if s.include}
              {@const v = sourceCheckView(sourceViews[s.app] ?? { status: 'idle' })}
              {@const check = sourceChecks[s.app]}
              <UrlField label="{meta.label} address" placeholder="http://{s.app}:port" required bind:value={s.url}
                onpaste={() => check.paste()} onblur={() => check.blur(full(s))}>
                {#snippet action()}
                  <Button size="sm" variant="secondary" disabled={!sourceCheckReady({ ...full(s), withKey: false })}
                    onclick={() => check.test({ ...full(s), withKey: false })}>Test<span class="visually-hidden"> the {meta.label} address</span></Button>
                {/snippet}
              </UrlField>
              <SecretField label="{meta.label} API key" help="Shown in the app's own settings. Used for this run only, never stored." required bind:value={s.apiKey}
                onpaste={() => check.paste()} onblur={() => check.blur(full(s))}>
                {#snippet action()}
                  <Button size="sm" variant="secondary" disabled={!sourceCheckReady(full(s))}
                    onclick={() => check.test(full(s))}>Test<span class="visually-hidden"> the {meta.label} address and key</span></Button>
                {/snippet}
                {#snippet result()}<InlineCheck family="sourceCheck" value={v?.value} detail={v?.detail} />{/snippet}
              </SecretField>
            {/if}
          </fieldset>
        {/each}
      </div>
    {:else if step.id === 'scope'}
      <MultiselectField label="Categories" options={categories} columns={2} bind:value={form.scope} />
      {#if form.scope.includes('history')}
        <NumberField label="History to keep" unit="days" min={1} max={365} bind:value={form.historyDays} help="Up to 365 days, newest first." />
      {/if}
      {@render configBanner()}
      <TextField label="Source path prefix" mono placeholder="/data/media" bind:value={form.fromPrefix}
        help="Only when the media folder is mounted elsewhere for corsarr. Files are checked on disk before they are linked."
        onpaste={() => pathCheck.paste()} onblur={() => pathCheck.blur(pathInput())}>
        {#snippet action()}{@render pathTest('the source path prefix')}{/snippet}
        {#snippet result()}{@render pathLine((c) => fromPrefixView(c, form.fromPrefix))}{/snippet}
      </TextField>
      <TextField label="corsarr path prefix" mono placeholder="/media" bind:value={form.toPrefix}
        onpaste={() => pathCheck.paste()} onblur={() => pathCheck.blur(pathInput())}>
        {#snippet action()}{@render pathTest('the corsarr path prefix')}{/snippet}
        {#snippet result()}{@render pathLine((c) => toPrefixView(c, form.toPrefix))}{/snippet}
      </TextField>
      <div class="sample" aria-live="polite">
        <p class="note">{form.fromPrefix.trim() || form.toPrefix.trim() ? 'Source root folders as corsarr will see them:' : 'No rewrite. Source root folders as they are on corsarr\'s side:'}</p>
        {#if pathCheckReady(pathInput())}{@render pathLine(sampleView)}
        {:else}<p class="note">Enter a reachable Sonarr, Radarr or Lidarr with its key to check paths.</p>{/if}
      </div>
      <BoolField label="Start imported kinds active" bind:checked={form.activate}
        help="Off: they start in shadow mode, so you can compare both side by side before corsarr grabs, imports or deletes anything." />
    {:else if step.id === 'plan'}
      {@render configBanner()}
      <SourceProgressStrip sources={progressOf(chosen, lines)} label="Sources read" unit="objects" />
      {#if running}<p class="note">Reading the sources…</p>{:else}
        <div class="row"><Button variant="secondary" icon="refresh" onclick={() => stream('plan')}>Build the plan again</Button></div>
      {/if}
      {@render table()}
    {:else}
      {#if result}
        <Banner preset="shadow" live>
          Imported kinds run in shadow mode unless you chose active, and every automation rule records what it would do without acting.
          Switch a kind to active beside these steps once corsarr agrees with your old setup.
        </Banner>
        {#if result.runId}<div class="row"><Button variant="secondary" icon="download" href="api/v1/migration/runs/{result.runId}/report">Download the report</Button></div>{/if}
        {@render table()}
      {:else}
        <p>Applying writes {applyCount(plan?.entries, form.exclude)} objects to the library, queue and history.
          Sources are only read again to check nothing moved since your review; downloads stay untouched in the client.</p>
        {#if running === 'apply'}<SourceProgressStrip sources={progressOf(chosen, lines)} label="Sources read" unit="objects" />{/if}
      {/if}
    {/if}
  {/snippet}
  {#snippet aside()}
    <section class="aside">
      <h2>Media kind modes</h2>
      {#each modes.data ?? [] as k (k.kind)}
        <div class="kind">
          <span class="kname">{kindName(k.kind)}</span>
          {#if k.managedByConfig}<ManagedValue value={modeName(k.mode)} managed={k.managedBy ?? managedFrom(`mediaKinds.${k.kind}.mode`, status.data?.configSource)} />
          {:else if k.mode === 'disabled'}<StatusBadge family="kindMode" value="disabled" size="sm" />
          {:else}
            <SegmentedControl label="{kindName(k.kind)} mode" size="sm" value={k.mode}
              options={[{ value: 'shadow', label: 'Shadow' }, { value: 'active', label: 'Active' }]} onchange={(m) => setMode(k.kind, m)} />
          {/if}
        </div>
      {/each}
      <p class="note">Before switching a kind to active, stop its old app or set it to unmonitored so both do not grab the same release.</p>
      <h2>Past runs</h2>
      {#each (runs.data ?? []).slice(0, 6) as r (r.id)}
        <p class="run"><Timestamp value={r.startedAt} /> · {r.mode === 'apply' ? 'Applied' : 'Plan'} · {r.sources.map(sourceName).join(', ')}
          {#if r.status === 'failed'}<StatusBadge family="check" value="fail" size="sm" />{/if}
          {#if r.status !== 'running'}<a href="api/v1/migration/runs/{r.id}/report">Report</a>{/if}</p>
      {:else}<p class="note">No runs yet.</p>{/each}
    </section>
  {/snippet}
</WizardPage>

<style>
  .sources { display: grid; gap: var(--space-5); }
  fieldset { display: grid; gap: var(--space-3); border: 0; padding: 0; margin: 0; }
  .tiles { display: grid; gap: var(--space-3); grid-template-columns: repeat(auto-fill, minmax(8rem, 1fr)); margin-block: var(--space-4); }
  .row { display: flex; gap: var(--space-2); margin-block: var(--space-3); }
  .obj { display: grid; min-width: 0; }
  .oname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .why { font-size: var(--text-sm); color: var(--ink-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .detail { display: grid; gap: var(--space-1); font-size: var(--text-sm); }
  .detail p { overflow-wrap: anywhere; }
  .note { font-size: var(--text-sm); color: var(--ink-muted); }
  .aside { display: grid; gap: var(--space-3); }
  .aside h2 { font-size: var(--text-md); color: var(--ink); }
  .kind { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
  .kname { color: var(--ink); }
  .run { font-size: var(--text-sm); display: flex; flex-wrap: wrap; gap: var(--space-1); align-items: center; }
  .sample { display: grid; gap: var(--space-1-5); }
  .notice { display: grid; gap: var(--space-2); margin: 0; padding-left: var(--space-4); }
</style>
