<script lang="ts">
  import { onMount } from 'svelte'
  import { router } from '../lib/router.svelte'
  import { problemMessage } from '../lib/api'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'

  let userCode = $state('')
  let error = $state('')
  let pending = $state(false)
  let success = $state(false)

  onMount(() => {
    const params = new URLSearchParams(router.search)
    const code = params.get('user_code') || params.get('code')
    if (code) {
      userCode = code
    }
  })

  async function submitDecision(approve: boolean) {
    error = ''
    pending = true
    try {
      const res = await fetch(`${router.base}/api/v1/auth/device/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ user_code: userCode.trim(), approve }),
      })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error(body ? problemMessage(body, res) : `Device authorization error (${res.status})`)
      }
      if (approve) {
        success = true
      } else {
        error = 'Device authorization denied.'
      }
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      pending = false
    }
  }
</script>

<div class="auth">
  <h1>Device Authorization</h1>
  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}

  {#if success}
    <div class="success" role="status">
      <p>Device authorized. You can close this window and return to your terminal.</p>
    </div>
  {:else}
    <form class="auth-form" onsubmit={(e) => { e.preventDefault(); submitDecision(true); }}>
      <p class="hint">Enter the code shown on your device or terminal.</p>
      <Input
        id="user_code"
        label="User Code"
        bind:value={userCode}
        required
        autocomplete="off"
        placeholder="ABCD-EFGH"
      />
      <div class="row-actions" style="margin-top: 1.5rem; display: flex; gap: 1rem;">
        <Button type="submit" variant="primary" disabled={pending || !userCode.trim()}>
          {pending ? 'Authorizing…' : 'Approve'}
        </Button>
        <Button type="button" variant="secondary" disabled={pending || !userCode.trim()} onclick={() => submitDecision(false)}>
          Deny
        </Button>
      </div>
    </form>
  {/if}
</div>