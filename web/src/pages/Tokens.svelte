<script lang="ts">
  import { onMount } from 'svelte'
  import { router } from '../lib/router.svelte'
  import { problemMessage } from '../lib/api'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'

  type PersonalToken = {
    id: number
    name: string
    created_at: string
    last_used_at?: string | null
    revoked_at?: string | null
  }

  type OAuthGrant = {
    id: number
    client_id: string
    client_name?: string
    scopes: string[]
    created_at: string
    last_used_at?: string | null
    revoked_at?: string | null
  }

  let tokens = $state<PersonalToken[]>([])
  let grants = $state<OAuthGrant[]>([])
  let loading = $state(true)
  let error = $state('')
  let newName = $state('')
  let createPending = $state(false)
  let oneTimeToken = $state<string | null>(null)
  let copied = $state(false)

  async function load() {
    error = ''
    try {
      const res = await fetch(`${router.base}/api/v1/me/tokens`, {
        credentials: 'same-origin',
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body ? problemMessage(body, res) : `Failed to load tokens (${res.status})`)
      }
      const data = await res.json()
      if (Array.isArray(data)) {
        tokens = data
      } else if (data) {
        tokens = data.tokens ?? data.personal_tokens ?? []
        grants = data.grants ?? data.oauth_grants ?? []
      }

      // Also try fetching /api/v1/oauth/grants if grants not populated or separate
      if (grants.length === 0) {
        const gRes = await fetch(`${router.base}/api/v1/oauth/grants`, {
          credentials: 'same-origin',
        })
        if (gRes.ok) {
          const gData = await gRes.json()
          if (Array.isArray(gData)) {
            grants = gData
          } else if (gData?.grants) {
            grants = gData.grants
          }
        }
      }
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      loading = false
    }
  }

  onMount(() => {
    void load()
  })

  async function createToken(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    copied = false
    oneTimeToken = null
    createPending = true
    try {
      const res = await fetch(`${router.base}/api/v1/me/tokens`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ name: newName.trim() }),
      })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error(body ? problemMessage(body, res) : `Failed to create token (${res.status})`)
      }
      if (body?.token) {
        oneTimeToken = body.token
      }
      newName = ''
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      createPending = false
    }
  }

  async function revokeToken(id: number) {
    error = ''
    try {
      const res = await fetch(`${router.base}/api/v1/me/tokens/${id}`, {
        method: 'DELETE',
        credentials: 'same-origin',
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body ? problemMessage(body, res) : `Failed to revoke token (${res.status})`)
      }
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    }
  }

  async function revokeGrant(id: number) {
    error = ''
    try {
      const res = await fetch(`${router.base}/api/v1/oauth/grants/${id}`, {
        method: 'DELETE',
        credentials: 'same-origin',
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body ? problemMessage(body, res) : `Failed to revoke grant (${res.status})`)
      }
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    }
  }

  async function copyToken() {
    if (!oneTimeToken) return
    try {
      await navigator.clipboard.writeText(oneTimeToken)
      copied = true
    } catch {
      error = 'Could not copy; select the token and copy it by hand.'
    }
  }
</script>

<h1>Tokens & Connected Apps</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

{#if oneTimeToken}
  <div class="success one-time" role="status">
    <span>
      New personal token created, shown once:
      <code class="secret">{oneTimeToken}</code>
    </span>
    <span class="hint">Store this token securely; it will not be shown again.</span>
    <Button variant="secondary" onclick={copyToken}>{copied ? 'Copied' : 'Copy'}</Button>
    <Button
      variant="secondary"
      onclick={() => {
        oneTimeToken = null
        copied = false
      }}>Dismiss</Button
    >
  </div>
{/if}

<section style="margin-bottom: 2rem;">
  <h2>Personal Tokens</h2>
  <form class="toolbar" onsubmit={createToken}>
    <Input id="token-name" label="Token Name" bind:value={newName} required placeholder="e.g., CLI or Script" />
    <Button type="submit" disabled={createPending}>
      {createPending ? 'Creating…' : 'Create token'}
    </Button>
  </form>

  {#if loading}
    <p>Loading tokens…</p>
  {:else if tokens.length === 0}
    <p>No personal tokens yet.</p>
  {:else}
    <table>
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Created</th>
          <th scope="col">Last Used</th>
          <th scope="col">Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each tokens as t (t.id)}
          <tr>
            <td>{t.name}</td>
            <td>{new Date(t.created_at).toLocaleString()}</td>
            <td>{t.last_used_at ? new Date(t.last_used_at).toLocaleString() : 'Never'}</td>
            <td>
              {#if !t.revoked_at}
                <Button variant="secondary" onclick={() => revokeToken(t.id)}>Revoke</Button>
              {:else}
                <span class="badge">Revoked</span>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>

<section>
  <h2>Connected OAuth Applications</h2>
  {#if loading}
    <p>Loading connected apps…</p>
  {:else if grants.length === 0}
    <p>No connected OAuth applications.</p>
  {:else}
    <table>
      <thead>
        <tr>
          <th scope="col">Application</th>
          <th scope="col">Scopes</th>
          <th scope="col">Authorized</th>
          <th scope="col">Last Used</th>
          <th scope="col">Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each grants as g (g.id)}
          <tr>
            <td>{g.client_name || g.client_id}</td>
            <td>{g.scopes?.join(', ') || 'none'}</td>
            <td>{new Date(g.created_at).toLocaleString()}</td>
            <td>{g.last_used_at ? new Date(g.last_used_at).toLocaleString() : 'Never'}</td>
            <td>
              {#if !g.revoked_at}
                <Button variant="secondary" onclick={() => revokeGrant(g.id)}>Revoke</Button>
              {:else}
                <span class="badge">Revoked</span>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>