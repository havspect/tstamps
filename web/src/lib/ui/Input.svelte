<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements'

  let {
    label,
    id,
    type = 'text',
    value = $bindable(''),
    checked = $bindable(false),
    hint = '',
    ...rest
  }: Omit<HTMLInputAttributes, 'value' | 'checked' | 'type'> & {
    label: string
    id: string
    type?: HTMLInputAttributes['type']
    value?: string
    checked?: boolean
    hint?: string
  } = $props()
</script>

{#if type === 'checkbox'}
  <wa-checkbox
    {id}
    {checked}
    hint={hint || undefined}
    onchange={(event) => (checked = event.currentTarget.checked)}
    {...rest}
  >
    {label}
  </wa-checkbox>
{:else if type === 'color'}
  <!-- Web Awesome has no plain color input; keep the native swatch. -->
  <div class="field">
    <label for={id}>{label}</label>
    <input {id} {type} value={value} oninput={(event) => (value = event.currentTarget.value)} {...rest} />
    {#if hint}<span class="hint">{hint}</span>{/if}
  </div>
{:else}
  <wa-input
    {id}
    {label}
    {type}
    hint={hint || undefined}
    value={value}
    oninput={(event) => (value = event.currentTarget.value)}
    {...rest}
  ></wa-input>
{/if}
