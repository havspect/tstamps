<script lang="ts">
  import { onMount } from 'svelte'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Link from '../lib/Link.svelte'

  const token = new URLSearchParams(location.search).get('token') ?? ''

  let email = $state('')
  let slug = $state('')
  let name = $state('')
  let password = $state('')
  let error = $state('')
  let loading = $state(true)
  let pending = $state(false)
  let invalid = $state(false)
  let disabled = $state(false)

  onMount(async () => {
    if (!token) {
      invalid = true
      loading = false
      return
    }
    try {
      const res = await fetch(`/api/v1/signup/verify?token=${encodeURIComponent(token)}`)
      if (res.status === 200) {
        email = (await res.json()).email
      } else {
        const problem = await res.json().catch(() => null)
        error = problem?.detail ?? 'This link is invalid or has already been used.'
        disabled = res.status === 503
        invalid = !disabled
      }
    } catch {
      error = 'Could not reach the server. Try again.'
      invalid = true
    } finally {
      loading = false
    }
  })

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    const workspace = slug.trim().toLowerCase()
    if (!/^[a-z0-9](?:[a-z0-9.-]{0,61}[a-z0-9])?$/.test(workspace) || workspace.includes('..')) {
      error = 'Enter a workspace URL such as acme.com.'
      return
    }
    pending = true
    try {
      const res = await fetch('/api/v1/signup/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, slug: workspace, name, password }),
      })
      if (res.status === 201) {
        const body = await res.json()
        location.assign(`/w/${encodeURIComponent(body.workspace)}/timeline`)
        return
      }
      const problem = await res.json().catch(() => null)
      error = problem?.detail ?? `Could not create the workspace (${res.status}).`
    } catch {
      error = 'Could not reach the server. Try again.'
    } finally {
      pending = false
    }
  }
</script>

{#if loading}
  <div class="auth">
    <h1>Checking your link…</h1>
  </div>
{:else if disabled}
  <div class="auth">
    <h1>Signup is not available</h1>
    <p class="error" role="alert">{error}</p>
    <p class="hint">Ask the operator of this server for an invitation instead.</p>
  </div>
{:else if invalid}
  <div class="auth">
    <h1>Link not usable</h1>
    <p class="error" role="alert">{error}</p>
    <p class="hint"><Link href="/signup">Start a new signup</Link> to get a fresh link.</p>
  </div>
{:else}
  <form class="auth" onsubmit={submit}>
    <h1>Create your workspace</h1>
    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}
    <p class="hint">
      Verified <strong>{email}</strong>. Choose the workspace URL people will use on this server,
      and the account that runs it.
    </p>
    <Input
      id="workspace"
      label="Workspace URL"
      bind:value={slug}
      placeholder="acme.com"
      required
      autofocus
      autocapitalize="none"
      autocomplete="organization"
      spellcheck={false}
    />
    <Input id="name" label="Your name" bind:value={name} required autocomplete="name" />
    <Input
      id="password"
      label="Password"
      type="password"
      bind:value={password}
      required
      minlength={8}
      autocomplete="new-password"
      hint="At least 8 characters."
    />
    <Button type="submit" disabled={pending}>
      {pending ? 'Creating…' : 'Create workspace'}
    </Button>
  </form>
{/if}
