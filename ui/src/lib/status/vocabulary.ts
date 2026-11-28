// The shared status vocabulary. Every status in the UI is rendered from this table:
// a text label plus an icon plus a tone; colour is never the only cue.
// Add a value here (and its story) before a screen shows it.
import type { Tone } from '../tokens';
import type { IconName } from '../components/atoms/Icon/icons';

export interface StatusEntry {
  label: string;
  icon: IconName;
  tone: Tone;
  /** The state is in motion (downloading, running): the badge may animate its icon. */
  active?: boolean;
  /** One plain sentence on what the state means, shown as the badge's tooltip. */
  hint?: string;
}

type Family = Record<string, StatusEntry>;
const e = (label: string, icon: IconName, tone: Tone, active?: boolean): StatusEntry =>
  active ? { label, icon, tone, active } : { label, icon, tone };

export const vocabulary = {
  download: {
    queued: e('Queued', 'clock', 'neutral'),
    downloading: e('Downloading', 'arrow-down', 'info', true),
    stalled: e('Stalled', 'pause-circle', 'warn'),
    completed: e('Completed', 'check', 'ok'),
    'import-pending': e('Import pending', 'tray', 'info'),
    importing: e('Importing', 'tray', 'info', true),
    'import-blocked': e('Import blocked', 'ban', 'error'),
    imported: e('Imported', 'check-circle', 'ok'),
    failed: e('Failed', 'x-circle', 'error'),
    removed: e('Removed', 'trash', 'neutral'),
  },
  downloadModifier: {
    paused: e('Paused', 'pause', 'neutral'),
    held: e('Held', 'pause', 'neutral'),
    shadow: e('Shadow', 'shadow', 'info'),
  },
  link: {
    pending: e('Pending', 'clock', 'neutral'),
    imported: e('Imported', 'check-circle', 'ok'),
    blocked: e('Blocked', 'ban', 'error'),
    skipped: e('Skipped', 'skip', 'neutral'),
    failed: e('Failed', 'x-circle', 'error'),
  },
  health: {
    ok: e('OK', 'check-circle', 'ok'),
    degraded: e('Degraded', 'alert-triangle', 'warn'),
    backed_off: e('Backed off', 'timer', 'warn'),
    failing: e('Failing', 'x-circle', 'error'),
    action_needed: e('Action needed', 'alert-octagon', 'error'),
    disabled: e('Disabled', 'minus-circle', 'neutral'),
  },
  verdict: {
    accept: e('Accepted', 'check', 'ok'),
    reject: e('Rejected', 'x', 'error'),
  },
  reason: {
    safety: { ...e('Safety', 'shield', 'error'), hint: 'Protects your machine and library. Never overridden.' },
    permanent: { ...e('Permanent', 'ban', 'error'), hint: 'Will not change by waiting: blocklisted, wrong item or below the profile floor. Overriding needs a confirmation.' },
    preference: { ...e('Preference', 'sliders', 'neutral'), hint: 'Your profile’s choices: size, formats, resolution cap. You can grab anyway.' },
    temporary: { ...e('Temporary', 'clock', 'warn'), hint: 'May pass later: seeders, a delay profile or an indexer limit. You can grab anyway.' },
  },
  automation: {
    grab: { ...e('Automation pick', 'bolt', 'ok'), hint: 'Automatic search would grab this release.' },
    shadow: { ...e('Automation pick (shadow)', 'shadow', 'info'), hint: 'Shadow mode: automatic search would grab this release, but only records it in History.' },
    outranked: { ...e('Accepted, not the pick', 'check', 'neutral'), hint: 'Passes the profile, but automatic search prefers a higher-ranked release.' },
    declined: { ...e('Rejected', 'x', 'error'), hint: 'Automatic search would skip this release.' },
    pending: { ...e('Deciding…', 'clock', 'neutral'), hint: 'The automation decision appears when every indexer has answered.' },
    unavailable: { ...e('Accepted', 'check', 'neutral'), hint: 'The search stopped early, so automation’s pick is unknown.' },
  },
  availability: {
    complete: { ...e('Complete', 'check-circle', 'ok'), hint: 'At least one full copy is available.' },
    may_be_incomplete: { ...e('May be incomplete', 'half', 'warn'), hint: 'No one is seeding a full copy; peers may not have every piece.' },
    no_peers: { ...e('No peers', 'x-circle', 'error'), hint: 'The indexer reports nobody sharing this torrent.' },
    unknown: { ...e('Availability unknown', 'help-circle', 'neutral'), hint: 'The indexer does not report completeness.' },
  },
  override: {
    allowed: e('Override allowed', 'unlock', 'neutral'),
    confirm: e('Override with confirm', 'alert-triangle', 'warn'),
    never: e('No override', 'lock', 'error'),
  },
  importOutcome: {
    imported: e('Imported', 'check-circle', 'ok'),
    upgraded: e('Upgraded', 'arrow-up', 'ok'),
    partial: e('Partial', 'half', 'warn'),
    skipped: e('Skipped', 'skip', 'neutral'),
    blocked: e('Blocked', 'ban', 'error'),
    failed: e('Failed', 'x-circle', 'error'),
  },
  ruleMode: {
    off: e('Off', 'minus-circle', 'neutral'),
    'dry-run': e('Dry run', 'eye', 'info'),
    live: e('Live', 'bolt', 'ok'),
  },
  classification: {
    healthy: e('Healthy', 'check-circle', 'ok'),
    grace: e('In grace', 'clock', 'neutral'),
    ambiguous: e('Ambiguous', 'help-circle', 'warn'),
    held: e('Held', 'pause', 'neutral'),
    'would-act': e('Would act', 'eye', 'info'),
    acted: e('Acted', 'bolt', 'info'),
    deferred: e('Deferred', 'clock', 'neutral'),
  },
  ruleBadge: {
    stalled: e('Stalled', 'pause-circle', 'warn'),
    'missing-files': e('Missing files', 'file-x', 'error'),
    'metadata-timeout': e('Metadata timeout', 'timer', 'warn'),
    'stuck-import': e('Stuck import', 'tray', 'error'),
  },
  planOp: {
    create: e('Create', 'plus', 'ok'),
    update: e('Update', 'pencil', 'info'),
    prune: e('Prune', 'minus', 'error'),
    skip: e('Skip', 'skip', 'neutral'),
    deferred: e('Deferred', 'clock', 'neutral'),
  },
  migration: {
    create: e('Create', 'plus', 'ok'),
    update: e('Update', 'pencil', 'info'),
    unchanged: e('Unchanged', 'equal', 'neutral'),
    skip: e('Skip', 'skip', 'neutral'),
    approximate: e('Approximate', 'approx', 'warn'),
    unsupported: e('Unsupported', 'ban', 'neutral'),
    conflict: e('Conflict', 'alert-triangle', 'error'),
    'secret-needed': e('Secret needed', 'key', 'warn'),
    'file-missing': e('File missing', 'file-x', 'error'),
    managed: e('Managed by config', 'lock', 'managed'),
  },
  item: {
    missing: e('Missing', 'minus-circle', 'warn'),
    queued: e('Queued', 'clock', 'info'),
    'cutoff-met': e('On disk', 'check-circle', 'ok'),
    'cutoff-unmet': e('Below cutoff', 'arrow-up', 'warn'),
    'not-available': e('Not available yet', 'calendar', 'neutral'),
    'outside-profile': e('Outside profile', 'ban', 'neutral'),
    unmonitored: e('Not monitored', 'eye-off', 'neutral'),
  },
  job: {
    queued: e('Queued', 'clock', 'neutral'),
    running: e('Running', 'refresh', 'info', true),
    done: e('Done', 'check', 'ok'),
    failed: e('Failed', 'x-circle', 'error'),
    cancelled: e('Cancelled', 'minus-circle', 'neutral'),
  },
  delivery: {
    pending: e('Pending', 'clock', 'neutral'),
    sending: e('Sending', 'arrow-right', 'info', true),
    delivered: e('Delivered', 'check', 'ok'),
    failed: e('Failed', 'x-circle', 'error'),
    dropped: e('Dropped', 'minus-circle', 'neutral'),
  },
  plugin: {
    ok: e('OK', 'check-circle', 'ok'),
    invalid: e('Invalid', 'x-circle', 'error'),
    incompatible: e('Incompatible', 'alert-triangle', 'warn'),
    excluded: e('Excluded', 'minus-circle', 'neutral'),
  },
  module: {
    active: { ...e('Active', 'check-circle', 'ok'), hint: 'Loaded and running for the instances that use it.' },
    failed: e('Failed to load', 'x-circle', 'error'),
    unloaded: { ...e('Unloaded', 'pause-circle', 'neutral'), hint: 'Installed but not loaded; its instances wait.' },
    uninstalled: { ...e('Uninstalled', 'minus-circle', 'neutral'), hint: 'Its instances stay in the config and run again once it is installed.' },
  },
  offer: {
    available: e('Available', 'download', 'neutral'),
    installed: e('Installed', 'check-circle', 'ok'),
    update: e('Update available', 'arrow-up', 'info'),
    update_needs_approval: { ...e('Update needs approval', 'shield', 'warn'), hint: 'The new version asks for more permissions, so it waits for you.' },
    older: e('Newer installed', 'check', 'neutral'),
    incompatible: e('Incompatible', 'ban', 'warn'),
  },
  actionOutcome: {
    running: e('Running', 'refresh', 'info', true),
    ok: e('OK', 'check-circle', 'ok'),
    warn: e('Warning', 'alert-triangle', 'warn'),
    fail: e('Failed', 'x-circle', 'error'),
    refused: e('Not shown', 'ban', 'neutral'),
  },
  kindMode: {
    disabled: { ...e('Disabled', 'minus-circle', 'neutral'), hint: 'Off: this kind is hidden from the library and never searched or downloaded.' },
    shadow: { ...e('Shadow', 'shadow', 'info'), hint: 'Dry run: corsarr searches and decides, and records what it would grab in History, but sends nothing to the download client and changes no files.' },
    active: { ...e('Active', 'check-circle', 'ok'), hint: 'Live: corsarr grabs releases, imports and renames files, and notifies.' },
  },
  check: {
    pass: e('Passed', 'check-circle', 'ok'),
    warn: e('Warning', 'alert-triangle', 'warn'),
    fail: e('Failed', 'x-circle', 'error'),
    running: e('Checking', 'refresh', 'info', true),
    pending: e('Not run', 'clock', 'neutral'),
    skipped: e('Skipped', 'skip', 'neutral'),
  },
  source: {
    pending: e('Waiting', 'clock', 'neutral'),
    running: e('Searching', 'search', 'info', true),
    ok: e('Answered', 'check', 'ok'),
    skipped: e('Skipped', 'skip', 'neutral'),
    failed: e('Failed', 'x-circle', 'error'),
  },
  sourceCheck: {
    checking: e('Checking…', 'refresh', 'info', true),
    reachable: e('Reachable', 'check-circle', 'ok'),
    'key-needed': e('Reachable, key needed', 'key', 'info'),
    unauthorised: e('Unauthorised', 'lock', 'error'),
    'wrong-app': e('Wrong app', 'alert-triangle', 'warn'),
    unsupported: e('Unsupported version', 'ban', 'warn'),
    unreachable: e('Unreachable', 'x-circle', 'error'),
  },
  connection: {
    live: e('Live', 'check-circle', 'ok'),
    reconnecting: e('Reconnecting', 'refresh', 'warn', true),
    offline: e('Disconnected', 'x-circle', 'error'),
  },
  secret: {
    set: e('Set', 'key', 'ok'),
    unset: e('Not set', 'minus-circle', 'neutral'),
    ref: e('From reference', 'link', 'info'),
  },
  pathAccess: {
    writable: e('Writable', 'check-circle', 'ok'),
    'read-only': e('Read-only', 'lock', 'warn'),
    unreachable: e('Unreachable', 'x-circle', 'error'),
  },
  config: {
    managed: e('Managed by config', 'lock', 'managed'),
    drifted: e('Drifted', 'diff', 'warn'),
  },
} as const satisfies Record<string, Family>;

export type StatusFamily = keyof typeof vocabulary;
export type StatusValue<F extends StatusFamily> = keyof (typeof vocabulary)[F] & string;

export function lookupStatus(family: StatusFamily, value: string): StatusEntry | undefined {
  return (vocabulary[family] as Family)[value];
}
