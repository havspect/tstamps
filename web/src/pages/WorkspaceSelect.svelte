<script lang="ts">
  import { onMount } from 'svelte'
  import Input from '../lib/ui/Input.svelte'
  import Button from '../lib/ui/Button.svelte'

  let slug = $state('')
  let error = $state('')
  let pending = $state(false)
  let checking = $state(true)

  onMount(async () => {
    // A client-side click on "Sign in" does not reload /login, so the server's
    // redirect cannot run. Ask whether there is exactly one workspace and go
    // straight to it; a multi-workspace server returns an empty slug.
    try {
      const res = await fetch('/api/v1/workspace')
      if (res.ok) {
        const body = (await res.json()) as { slug?: string }
        if (body.slug) {
          location.replace(`/w/${encodeURIComponent(body.slug)}/login`)
          return
        }
      }
    } catch {
      // Offline: show the prompt rather than guess.
    }
    checking = false
  })

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    const name = slug.trim().toLowerCase()
    if (!/^[a-z0-9](?:[a-z0-9.-]{0,61}[a-z0-9])?$/.test(name) || name.includes('..')) {
      error = 'Enter a workspace such as example.com.'
      return
    }
    pending = true
    try {
      // The host answers 404 for a slug with no database. Ask first, so a typo
      // stays on this form instead of replacing it with the bare error page.
      const res = await fetch(`/w/${encodeURIComponent(name)}/login`, { method: 'HEAD' })
      if (res.status === 404) {
        error = `Workspace ${name} does not exist on this server.`
        return
      }
    } catch {
      // Offline: navigation will show whatever the server can answer.
    } finally {
      pending = false
    }
    location.assign(`/w/${encodeURIComponent(name)}/login`)
  }
</script>

{#if !checking}
<form class="auth" onsubmit={submit}>
  <h1>Open your workspace</h1>
  {#if error}
    <wa-callout variant="danger" role="alert">{error}</wa-callout>
  {/if}
  <Input
    id="workspace"
    label="Workspace"
    bind:value={slug}
    placeholder="example.com"
    required
    autofocus
    autocapitalize="none"
    autocomplete="organization"
    spellcheck={false}
  />
  <Button type="submit" disabled={pending}>{pending ? 'Checking…' : 'Continue'}</Button>
</form>
{/if}
