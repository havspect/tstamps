<script lang="ts">
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Link from '../lib/Link.svelte'

  let email = $state('')
  let error = $state('')
  let pending = $state(false)
  let sent = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    pending = true
    try {
      const res = await fetch('/api/v1/signup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      })
      if (res.status === 202) {
        sent = true
        return
      }
      const problem = await res.json().catch(() => null)
      error = problem?.detail ?? `Signup failed (${res.status}).`
    } catch {
      error = 'Could not reach the server. Try again.'
    } finally {
      pending = false
    }
  }
</script>

{#if sent}
  <div class="auth">
    <h1>Check your email</h1>
    <p>
      We sent a verification link to <strong>{email}</strong>. Open it to choose your workspace
      and password.
    </p>
    <p class="hint">The link works once. Nothing arrived? Check spam, or start again.</p>
    <Button variant="secondary" onclick={() => (sent = false)}>Use another address</Button>
  </div>
{:else}
  <form class="auth" onsubmit={submit}>
    <h1>Create a workspace</h1>
    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}
    <p class="hint">
      Enter your email and we will send a link. The next page asks for the workspace URL, your
      name and a password.
    </p>
    <Input
      id="email"
      label="Email"
      type="email"
      bind:value={email}
      required
      autofocus
      autocomplete="email"
    />
    <Button type="submit" disabled={pending}>{pending ? 'Sending…' : 'Send link'}</Button>
    <p class="hint">Already have a workspace? <Link href="/login">Sign in</Link>.</p>
  </form>
{/if}
