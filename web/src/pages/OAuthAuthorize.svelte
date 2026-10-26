<script lang="ts">
  import { onMount } from 'svelte'
  import { router } from '../lib/router.svelte'
  import { session } from '../lib/session.svelte'
  import { problemMessage } from '../lib/api'
  import Button from '../lib/ui/Button.svelte'

  let clientName = $state('')
  let clientId = $state('')
  let scopes = $state<string[]>([])
  let loading = $state(true)
  let error = $state('')
  let pending = $state(false)

  const workspace = $derived(router.base.replace('/w/', '') || 'default')

  const scopeDescriptions: Record<string, string> = {
    'tstamps.read': 'Read your profile, projects, entries, and reports',
    'tstamps.write': 'Start and stop timers, and log or edit entries',
  }

  onMount(async () => {
    try {
      const res = await fetch(`${router.base}/api/v1/oauth/authorize${router.search}`, {
        credentials: 'same-origin',
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        const msg = body ? problemMessage(body, res) : `Authorization error (${res.status})`
        throw new Error(msg)
      }
      const data = await res.json()
      clientId = data.client_id ?? ''
      clientName = data.client_name ?? clientId
      scopes = data.scopes ?? []
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      loading = false
    }
  })

  async function submitDecision(approve: boolean) {
    error = ''
    pending = true
    try {
      const res = await fetch(`${router.base}/api/v1/oauth/authorize${router.search}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ approve }),
      })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error(body ? problemMessage(body, res) : `Authorization error (${res.status})`)
      }
      if (body?.redirect_to) {
        window.location.replace(body.redirect_to)
      } else {
        throw new Error('Missing redirect_to in response')
      }
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
      pending = false
    }
  }
</script>

<div class="auth">
  <h1>Authorize Application</h1>
  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}

  {#if loading}
    <p>Loading authorization request…</p>
  {:else if !error}
    <div class="auth-details">
      <p>
        <strong>{clientName || clientId}</strong> ({clientId}) wants access to your workspace <strong>{workspace}</strong>.
      </p>
      {#if session.user}
        <p class="hint">Signed in as <strong>{session.user.email}</strong></p>
      {/if}

      {#if scopes.length > 0}
        <div class="scopes-section" style="margin: 1rem 0;">
          <h3>Requested permissions:</h3>
          <ul>
            {#each scopes as s}
              <li>
                <strong>{s}</strong>: {scopeDescriptions[s] || s}
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      <div class="row-actions" style="margin-top: 1.5rem; display: flex; gap: 1rem;">
        <Button variant="primary" disabled={pending} onclick={() => submitDecision(true)}>
          {pending ? 'Processing…' : 'Approve'}
        </Button>
        <Button variant="secondary" disabled={pending} onclick={() => submitDecision(false)}>
          Deny
        </Button>
      </div>
    </div>
  {/if}
</div>