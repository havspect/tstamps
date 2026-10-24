<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'

  let {
    variant = 'primary',
    active,
    type = 'button',
    class: className = '',
    children,
    ...rest
  }: HTMLButtonAttributes & {
    variant?: 'primary' | 'secondary' | 'danger'
    active?: boolean
    children: Snippet
  } = $props()
</script>

<wa-button
  variant={variant === 'danger' ? 'danger' : variant === 'primary' || active ? 'brand' : 'neutral'}
  appearance={active ? 'filled' : variant === 'secondary' ? 'outlined' : 'accent'}
  {type}
  size="s"
  class={className}
  {...rest}
>
  {@render children()}
  {#if active}<span class="sr-only"> (selected)</span>{/if}
</wa-button>
