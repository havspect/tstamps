<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLSelectAttributes } from 'svelte/elements'

  let {
    label,
    id,
    value = $bindable(''),
    hint = '',
    children,
    ...rest
  }: Omit<HTMLSelectAttributes, 'value' | 'size'> & {
    label: string
    id: string
    value?: string
    hint?: string
    children: Snippet
  } = $props()
</script>

<wa-select
  {id}
  {label}
  hint={hint || undefined}
  {value}
  size="s"
  oninput={(event) => (value = event.currentTarget.value)}
  {...rest}
>
  {@render children()}
</wa-select>
