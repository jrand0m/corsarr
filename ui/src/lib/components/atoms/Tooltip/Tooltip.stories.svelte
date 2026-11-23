<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Tooltip from './Tooltip.svelte';
  import Button from '../Button/Button.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Tooltip', component: Tooltip, tags: ['autodocs'] });
</script>

<Story name="Hover or focus" asChild>
  <ThemePair>
    <Row label="Tab to a trigger, or hover; Esc hides">
      <Tooltip text="22 Nov 2026, 14:05">
        {#snippet children(trigger)}<Button {...trigger}>Last searched 5m ago</Button>{/snippet}
      </Tooltip>
      <Tooltip text="Opens below near the top of the viewport" placement="bottom">
        {#snippet children(trigger)}<Button variant="ghost" {...trigger}>Below</Button>{/snippet}
      </Tooltip>
    </Row>
  </ThemePair>
</Story>
