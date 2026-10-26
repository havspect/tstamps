<script lang="ts">
  import { session } from '../lib/session.svelte'
  import { router } from '../lib/router.svelte'
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
