// Wire types of a plugin module's settings schema, permissions and action results, shared by the
// form converter, the plugin components and the API client.

/** Field types of a module's settings schema: a closed set the server enforces. */
export type ModuleFieldType =
  | 'string' | 'secret' | 'bool' | 'int' | 'number' | 'enum' | 'duration'
  | 'list<string>' | 'list<int>' | 'map<string,string>';

/** Fixed string patterns a module may ask for. */
export type ModulePattern = 'hostname' | 'url-http' | 'path-absolute' | 'identifier';

export interface ModuleTextRule { minLength?: number; maxLength?: number; pattern?: ModulePattern }

export interface ModuleField {
  key: string;
  /** Typed as string: anything outside ModuleFieldType is refused by the converter. */
  type: string;
  label: string;
  description?: string;
  /** Plain-text hint shown with a validation message. */
  hint?: string;
  required?: boolean;
  default?: unknown;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  min?: number;
  max?: number;
  step?: number;
  decimals?: number;
  options?: { value: string; label: string }[];
  /** Go durations: "30s", "5m". */
  minDuration?: string;
  maxDuration?: string;
  maxItems?: number;
  maxEntries?: number;
  keyRule?: ModuleTextRule;
  valueRule?: ModuleTextRule;
}

export interface ModuleGroup { title: string; description?: string; fields: string[] }
export interface ModuleAction { id: string; label: string }
export interface ModuleSchema { fields: ModuleField[]; groups?: ModuleGroup[]; actions?: ModuleAction[] }

/** Hosts a module may reach ("*.example.org", "$setting:url") and the secret fields it may read. */
export interface ModulePermissions { http: { hosts?: string[] }; secrets?: string[] }

/** A configured plugin instance that uses a module. */
export interface InstanceRef { section: string; key: string; name: string; enabled: boolean }

export interface PermissionDiff { addedHosts: string[]; removedHosts: string[]; addedSecrets: string[]; removedSecrets: string[] }

export type ActionOutcome = 'ok' | 'warn' | 'fail';
export interface ActionResult { outcome: ActionOutcome; text: string }

/** One validation failure; the server spells the text `message` (older answers `msg`). */
export interface Problem { path: string; message?: string; msg?: string }
