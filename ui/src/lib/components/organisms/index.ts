// Organisms barrel: molecules and atoms composed into screen regions. Import from '../organisms'.

// Lane B: overlays.
export { default as Dialog } from './Dialog/Dialog.svelte';
export { default as Drawer } from './Drawer/Drawer.svelte';
export { default as ConfirmDialog } from './ConfirmDialog/ConfirmDialog.svelte';
export { default as Wizard } from './Wizard/Wizard.svelte';
export { default as ToastRegion } from './ToastRegion/ToastRegion.svelte';
export { ToastQueue, type ToastInput, type ToastEntry } from '../../toast/toasts.svelte';
export type { DialogCloseReason } from './Dialog/Dialog.svelte';
export type { ConfirmChoice } from './ConfirmDialog/ConfirmDialog.svelte';
export type { WizardStep } from './Wizard/Wizard.svelte';

// Lane B: explanation and decision.
export { default as EvidencePanel } from './EvidencePanel/EvidencePanel.svelte';
export { default as DecisionPanel, overrideLevel, mainReason } from './DecisionPanel/DecisionPanel.svelte';
export { default as ReleaseResults } from './ReleaseResults/ReleaseResults.svelte';
export type { Evidence, Threshold, ReasonCategory } from './EvidencePanel/EvidencePanel.svelte';
export type { AutomationOutcome, Decision, DecisionReason, OverrideLevel } from './DecisionPanel/DecisionPanel.svelte';

// Lane B: headers, navigation and picking.
export { default as EntityHeader } from './EntityHeader/EntityHeader.svelte';
export { default as CommandPalette } from './CommandPalette/CommandPalette.svelte';
export { default as VariantPicker } from './VariantPicker/VariantPicker.svelte';
export type { EntityStatus, EntityFact } from './EntityHeader/EntityHeader.svelte';
export type { PaletteItem } from './CommandPalette/CommandPalette.svelte';
export type { Variant } from './VariantPicker/VariantPicker.svelte';

// PlanDiff is heavy for first load: load it on demand, never import it statically into a route.
//   {#await loadPlanDiff() then PlanDiff}<PlanDiff … />{/await}
export const loadPlanDiff = () => import('./PlanDiff/PlanDiff.svelte').then((m) => m.default);
export type { PlanGroup, PlanOp, PlanField, PlanOpKind } from './PlanDiff/PlanDiff.svelte';

// Lane A: data and forms.
export { default as SchemaForm } from './SchemaForm/SchemaForm.svelte';
export { default as PluginSettingsForm } from './PluginSettingsForm/PluginSettingsForm.svelte';
export { default as PluginInstallDialog } from './PluginInstallDialog/PluginInstallDialog.svelte';
export type { ActionReply } from './PluginSettingsForm/PluginSettingsForm.svelte';
export type { ModuleChange } from './PluginInstallDialog/PluginInstallDialog.svelte';
export { default as SetupFlow } from './SetupFlow/SetupFlow.svelte';
export { default as OrderedListEditor } from './OrderedListEditor/OrderedListEditor.svelte';
export { default as ToggleMatrix } from './ToggleMatrix/ToggleMatrix.svelte';
export { default as LiveTestBox } from './LiveTestBox/LiveTestBox.svelte';
export { default as FilterBar } from './FilterBar/FilterBar.svelte';
export { default as DataTable } from './DataTable/DataTable.svelte';
export { default as PosterGrid } from './PosterGrid/PosterGrid.svelte';
export { default as Timeline } from './Timeline/Timeline.svelte';
export type { SetupDriver, SetupCode, SetupOutcome } from './SetupFlow/SetupFlow.svelte';
export type {
  SettingsSchema, SchemaField, SchemaFieldType, SchemaOption, SchemaGroup, SchemaCondition, SchemaValidation,
  SetupFlowSpec, SetupFlowKind, FieldMeta, FormValues,
} from '../../forms/schema';
export type { OrderedItem } from './OrderedListEditor/OrderedListEditor.svelte';
export type { MatrixColumn, MatrixRow, MatrixValue, MatrixCell } from './ToggleMatrix/ToggleMatrix.svelte';
export type { TestResult, TestField, TestCondition } from './LiveTestBox/LiveTestBox.svelte';
export type { FilterFacet, FilterOption, FilterState } from '../../filters/url';
export { filterToParams, filterFromParams, emptyFilter } from '../../filters/url';
export type { Column, SortState } from './DataTable/DataTable.svelte';
export type { PosterItem } from './PosterGrid/PosterGrid.svelte';
export type { TimelineEntry, TimelineKind, TimelineStatus } from './Timeline/Timeline.svelte';

// Calendar and LogViewer are heavy for first load: load them on demand, never import them statically.
//   {#await loadCalendar() then Calendar}<Calendar … />{/await}
export const loadCalendar = () => import('./Calendar/Calendar.svelte').then((m) => m.default);
export const loadLogViewer = () => import('./LogViewer/LogViewer.svelte').then((m) => m.default);
export type { CalendarEvent, CalendarView } from './Calendar/Calendar.svelte';
export type { LogLine, LogLevel } from '../../format/log';
