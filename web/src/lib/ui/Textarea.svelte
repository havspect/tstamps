<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements'

  let {
    label,
    id,
    value = $bindable(''),
    hint = '',
    ...rest
  }: Omit<HTMLTextareaAttributes, 'value'> & {
    label: string
    id: string
    value?: string
    hint?: string
  } = $props()
</script>

<wa-textarea
  {id}
  {label}
  hint={hint || undefined}
  {value}
  oninput={(event) => (value = event.currentTarget.value)}
  {...rest}
></wa-textarea>
