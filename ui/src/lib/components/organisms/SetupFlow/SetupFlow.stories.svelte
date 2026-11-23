<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SetupFlow, { type SetupDriver } from './SetupFlow.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { SetupFlowSpec } from '../../../forms/schema';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const pause = (ms: number) => new Promise((r) => setTimeout(r, ms));
  const never = () => new Promise<never>(() => {});
  const pin: SetupFlowSpec = { kind: 'pinPairing', label: 'Pair with Plex', help: 'corsarr shows a code; enter it at plex.tv/link while signed in.', fills: ['token'] };
  const device: SetupFlowSpec = { kind: 'oauthDevice', label: 'Connect a Trakt account', help: 'Approve corsarr on Trakt with a one-time code.', fills: ['accessToken', 'refreshToken'] };
  const cookie: SetupFlowSpec = { kind: 'cookiePaste', label: 'Use a browser session', fills: ['cookie'] };
  const login: SetupFlowSpec = { kind: 'loginForm', label: 'Sign in to the tracker', help: 'Trades your password for a session cookie once.', fills: ['cookie'], otp: true };

  const waiting: SetupDriver = {
    start: async () => ({ code: 'K7QP', url: 'https://plex.tv/link', expiresAt: now + 14 * 60_000 }),
    wait: never,
  };
  const deviceDriver: SetupDriver = {
    start: async () => ({ code: 'B4XD-91MT', url: 'https://trakt.tv/activate', expiresAt: now + 9 * 60_000 }),
    wait: never,
  };
  const pairs: SetupDriver = {
    start: async () => { await pause(300); return { code: 'K7QP', url: 'https://plex.tv/link', expiresAt: now + 14 * 60_000 }; },
    wait: async () => { await pause(3000); return { ok: true, message: 'Paired with Plex as harbourmaster. The token is saved.' }; },
    submit: async (_f, input) => { await pause(500); return input.password === 'wrong' ? { ok: false, error: 'The tracker refused those credentials.' } : { ok: true, message: 'Signed in. The session cookie is saved.' }; },
  };
  const refused: SetupDriver = {
    start: async () => { throw new Error('plex.tv did not answer (timeout after 10s). Try again in a minute.'); },
    submit: async () => ({ ok: false, error: 'The tracker refused those credentials.' }),
  };

  const { Story } = defineMeta({ title: 'Organisms/SetupFlow', component: SetupFlow, tags: ['autodocs'], args: { flow: pin, driver: pairs, now } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="PIN pairing, idle"><SetupFlow flow={pin} driver={pairs} {now} /></Row>
    <Row label="PIN pairing, waiting (press Get a code)"><SetupFlow flow={pin} driver={waiting} {now} /></Row>
    <Row label="Device code, waiting (press Get a code)"><SetupFlow flow={device} driver={deviceDriver} {now} /></Row>
    <Row label="Start failed (press Get a code)"><SetupFlow flow={pin} driver={refused} {now} /></Row>
    <Row label="Cookie paste"><SetupFlow flow={cookie} driver={pairs} /></Row>
    <Row label="Login form with one-time code (type 'wrong' as the password)"><SetupFlow flow={login} driver={pairs} /></Row>
    <Row label="Done"><SetupFlow flow={pin} driver={pairs} done /></Row>
    <Row label="Disabled"><SetupFlow flow={login} driver={pairs} disabled /></Row>
  </ThemePair>
</Story>
