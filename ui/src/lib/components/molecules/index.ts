// Molecules barrel: atoms composed into one-purpose units. Import from '../molecules'.
// SchemaForm field molecules: Field is the shared wrapper; one molecule per field type.
export { default as Field } from './Field/Field.svelte';
export { default as TextField } from './TextField/TextField.svelte';
export { default as SecretField } from './SecretField/SecretField.svelte';
export { default as NumberField } from './NumberField/NumberField.svelte';
export { default as BoolField } from './BoolField/BoolField.svelte';
export { default as EnumField } from './EnumField/EnumField.svelte';
export { default as MultiselectField } from './MultiselectField/MultiselectField.svelte';
export { default as DurationField } from './DurationField/DurationField.svelte';
export { default as SizeField } from './SizeField/SizeField.svelte';
export { default as UrlField } from './UrlField/UrlField.svelte';
export { default as PathField } from './PathField/PathField.svelte';
export { default as RegexField } from './RegexField/RegexField.svelte';
export { default as ListField } from './ListField/ListField.svelte';
export { default as MapField } from './MapField/MapField.svelte';
export type { FieldControl, FieldStateProps } from './Field/Field.svelte';
export type { MapEntry } from './MapField/MapField.svelte';

// Pattern molecules.
export { default as PathPicker } from './PathPicker/PathPicker.svelte';
export type { PathLocation } from './PathPicker/PathPicker.svelte';
export { default as ItemRefSummary } from './ItemRefSummary/ItemRefSummary.svelte';
export { default as PropertyList } from './PropertyList/PropertyList.svelte';
export { default as CodeViewer } from './CodeViewer/CodeViewer.svelte';
export { default as StatTile } from './StatTile/StatTile.svelte';
export { default as CheckResults } from './CheckResults/CheckResults.svelte';
export { default as SourceProgressStrip } from './SourceProgressStrip/SourceProgressStrip.svelte';
export { default as Banner } from './Banner/Banner.svelte';
export { default as EmptyState } from './EmptyState/EmptyState.svelte';
export { default as ErrorState } from './ErrorState/ErrorState.svelte';
export type { ItemRef } from './ItemRefSummary/ItemRefSummary.svelte';
export type { Property } from './PropertyList/PropertyList.svelte';
export type { CodeLanguage } from './CodeViewer/CodeViewer.svelte';
export type { CheckResult } from './CheckResults/CheckResults.svelte';
export type { SourceProgress } from './SourceProgressStrip/SourceProgressStrip.svelte';
export type { BannerPreset } from './Banner/Banner.svelte';
export { default as AsyncPicker } from './AsyncPicker/AsyncPicker.svelte';
export { default as TemplateInput } from './TemplateInput/TemplateInput.svelte';
export { default as MonitorToggle } from './MonitorToggle/MonitorToggle.svelte';
export { default as BulkActionBar } from './BulkActionBar/BulkActionBar.svelte';
export { default as Tabs } from './Tabs/Tabs.svelte';
export { default as SegmentedControl } from './SegmentedControl/SegmentedControl.svelte';
export { default as Toast } from './Toast/Toast.svelte';
export { default as GlobalStatus } from './GlobalStatus/GlobalStatus.svelte';
export { default as ActionMenu } from './ActionMenu/ActionMenu.svelte';
export { default as QueueCard } from './QueueCard/QueueCard.svelte';
export type { PickerOption, PickerSearch } from './AsyncPicker/AsyncPicker.svelte';
export type { TemplateToken, TemplateSample } from './TemplateInput/TemplateInput.svelte';
export type { MonitorState } from './MonitorToggle/MonitorToggle.svelte';
export type { TabItem } from './Tabs/Tabs.svelte';
export type { Segment } from './SegmentedControl/SegmentedControl.svelte';
export type { ActionItem } from './ActionMenu/ActionMenu.svelte';
export type { QueueCardData, QueueAction } from './QueueCard/QueueCard.svelte';

// TimeSeries is heavy for first load: load it on demand, never import it statically into a route.
//   {#await loadTimeSeries() then TimeSeries}<TimeSeries … />{/await}
export const loadTimeSeries = () => import('./TimeSeries/TimeSeries.svelte').then((m) => m.default);
export type { TimeBucket, TimeRange } from './TimeSeries/TimeSeries.svelte';
export { default as NewPasswordField, passwordStrength } from './NewPasswordField/NewPasswordField.svelte';
