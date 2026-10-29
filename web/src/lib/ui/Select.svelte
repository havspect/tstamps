<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLSelectAttributes } from 'svelte/elements'

  let {
    label,
    id,
    value = $bindable(''),
    hint = '',
    size = 's',
    children,
    ...rest
  }: Omit<HTMLSelectAttributes, 'value' | 'size'> & {
    label: string
    id: string
    value?: string
    hint?: string
    size?: 's' | 'm' | 'l'
    children: Snippet
  } = $props()
</script>

<wa-select
  {id}
  {label}
  {size}
  hint={hint || undefined}
  {value}
  oninput={(event) => (value = event.currentTarget.value)}
  {...rest}
>
  {@render children()}
</wa-select>
