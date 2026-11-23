<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NewPasswordField from './NewPasswordField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Molecules/NewPasswordField',
    component: NewPasswordField,
    tags: ['autodocs'],
    args: { label: 'Password', minLength: 12 },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Empty"><div class="w"><NewPasswordField /></div></Row>
    <Row label="Too short"><div class="w"><NewPasswordField value="harbour" /></div></Row>
    <Row label="Fair, with a hint"><div class="w"><NewPasswordField value="harbourmaster" /></div></Row>
    <Row label="Strong and confirmed"><div class="w"><NewPasswordField value="Quay-lights-7-after-dark" confirm="Quay-lights-7-after-dark" /></div></Row>
    <Row label="Confirm does not match"><div class="w"><NewPasswordField value="Quay-lights-7-after-dark" confirm="Quay-lights-7-after-dusk" /></div></Row>
    <Row label="After a submit attempt with nothing typed"><div class="w"><NewPasswordField showErrors /></div></Row>
    <Row label="Server error"><div class="w"><NewPasswordField value="password1234" confirm="password1234" error="This password appears in a list of leaked passwords. Choose another." /></div></Row>
    <Row label="Disabled while saving"><div class="w"><NewPasswordField value="Quay-lights-7-after-dark" confirm="Quay-lights-7-after-dark" disabled /></div></Row>
  </ThemePair>
</Story>

<style>
  .w { width: 100%; max-width: 24rem; }
</style>
