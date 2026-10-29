<script lang="ts">
  import { session } from '../lib/session.svelte'
  import { router } from '../lib/router.svelte'
  import { demo } from '../lib/demo'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'

  let email = $state('')
  let password = $state('')
  let error = $state('')
  let pending = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    pending = true
    try {
      await session.login(email, password)
      const params = new URLSearchParams(router.search)
      const next = params.get('next')
      if (next && next.startsWith('/') && !next.startsWith('//')) {
        router.navigate(next, { replace: true })
      } else {
        router.navigate('/timeline', { replace: true })
      }
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      pending = false
    }
  }
</script>

<form class="auth" onsubmit={submit}>
  <h1>Sign in</h1>
  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}
  {#if demo.accounts.length}
    <div class="demo-accounts">
      <p class="hint">Demo accounts — click one to fill the form.</p>
      {#each demo.accounts as account (account.email)}
        <button
          type="button"
          class="demo-account"
          onclick={() => {
            email = account.email
            password = account.password
          }}
        >
          <strong>{account.name}</strong>
          <span>{account.role} · {account.email}</span>
        </button>
      {/each}
    </div>
  {/if}
  <Input id="email" label="Email" type="email" bind:value={email} required autocomplete="username" />
  <Input
    id="password"
    label="Password"
    type="password"
    bind:value={password}
    required
    autocomplete="current-password"
  />
  <Button type="submit" disabled={pending}>
    {pending ? 'Signing in…' : 'Sign in'}
  </Button>
</form>

<style>
  .demo-accounts {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
  }

  .demo-accounts .hint {
    margin: 0;
  }

  .demo-account {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    gap: 2px;
    height: auto;
    padding: var(--space-2) var(--space-3);
    color: var(--text);
    text-align: left;
    white-space: normal;
    background: var(--surface);
    border-color: var(--border);
  }

  .demo-account:hover {
    border-color: var(--accent);
  }

  .demo-account span {
    color: var(--muted);
    font-size: 0.8125rem;
  }
</style>
