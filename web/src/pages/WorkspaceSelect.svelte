<script lang="ts">
  import Input from '../lib/ui/Input.svelte'
  import Button from '../lib/ui/Button.svelte'

  let slug = $state('')
  let error = $state('')

  function submit(event: SubmitEvent) {
    event.preventDefault()
    const name = slug.trim().toLowerCase()
    if (!/^[a-z0-9](?:[a-z0-9.-]{0,61}[a-z0-9])?$/.test(name) || name.includes('..')) {
      error = 'Enter a workspace such as koeber.com.'
      return
    }
    location.assign(`/w/${encodeURIComponent(name)}/login`)
  }
</script>

<form class="auth" onsubmit={submit}>
  <h1>Open your workspace</h1>
  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}
  <Input
    id="workspace"
    label="Workspace"
    bind:value={slug}
    placeholder="koeber.com"
    required
    autofocus
    autocapitalize="none"
    autocomplete="organization"
    spellcheck={false}
  />
  <Button type="submit">Continue</Button>
</form>
