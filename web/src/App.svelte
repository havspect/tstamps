<script lang="ts">
  import { onMount } from 'svelte'
  import type { Component } from 'svelte'
  import { router } from './lib/router.svelte'
  import { session } from './lib/session.svelte'
  import Link from './lib/Link.svelte'
  import Button from './lib/ui/Button.svelte'
  import Home from './pages/Home.svelte'
  import Login from './pages/Login.svelte'
  import NotFound from './pages/NotFound.svelte'
  import Password from './pages/Password.svelte'
  import Reports from './pages/Reports.svelte'
  import WorkspaceSelect from './pages/WorkspaceSelect.svelte'
  import Timeline from './pages/Timeline.svelte'
  import Signup from './pages/Signup.svelte'
  import SignupVerify from './pages/SignupVerify.svelte'
  import AdminEntries from './pages/AdminEntries.svelte'
  import AdminProjects from './pages/AdminProjects.svelte'
  import AdminTeams from './pages/AdminTeams.svelte'
  import AdminUsers from './pages/AdminUsers.svelte'

  const routes: Record<string, Component> = {
    '/': Home,
    '/login': Login,
    '/password': Password,
    '/signup': Signup,
    '/signup/verify': SignupVerify,
    '/timeline': Timeline,
    '/reports': Reports,
    '/admin/users': AdminUsers,
    '/admin/teams': AdminTeams,
    '/admin/projects': AdminProjects,
    '/admin/entries': AdminEntries,
  }

  let ready = $state(false)
  // A hosted server without /w/{slug} in the URL has no workspace yet: "/" is
  // the public homepage, "/login" asks for the slug, and the signup pages create
  // a workspace before one exists.
  const hostedRoot = $derived(router.hosted && router.base === '')
  const selectingWorkspace = $derived(hostedRoot && router.path === '/login')
  const hostedPublic = $derived(
    hostedRoot &&
      (router.path === '/' ||
        router.path === '/login' ||
        router.path === '/signup' ||
        router.path === '/signup/verify'),
  )

  onMount(async () => {
    if (!hostedRoot) {
      try {
        await session.load()
      } catch {
        // Offline: show the login screen rather than guess at a session.
      }
    }
    ready = true
  })

  const user = $derived(session.user)
  const leads = $derived(user?.memberships?.some((m) => m.role === 'lead') ?? false)
  const Page = $derived(routes[router.path] ?? NotFound)
  // Do not mount a page the shell is about to redirect away from: it would
  // fetch with the wrong role and flash its content for a frame. The homepage
  // is public, so it skips this check entirely.
  const allowed = $derived.by(() => {
    if (hostedRoot) return hostedPublic
    if (!user) return router.path === '/login' || (router.path === '/' && !router.hosted)
    if (session.mustChangePassword) return router.path === '/password'
    if (router.path === '/login' || router.path === '/password' || (router.path === '/' && router.hosted)) return false
    if (router.path.startsWith('/admin')) return user.is_admin
    if (router.path === '/reports') return user.is_admin || leads
    return true
  })

  // One guard for the whole shell: no session -> login; a reset password ->
  // the password screen; a signed-in user on an auth screen -> the timeline.
  // The homepage is public for everyone, signed in or not.
  $effect(() => {
    if (!ready) return
    const path = router.path
    if (hostedRoot) {
      if (!hostedPublic) router.navigate('/', { replace: true })
    } else if (!user) {
      const publicHome = path === '/' && !router.hosted
      if (path !== '/login' && !publicHome) router.navigate('/login', { replace: true })
    } else if (session.mustChangePassword) {
      if (path !== '/password') router.navigate('/password', { replace: true })
    } else if (path.startsWith('/admin') && !user.is_admin) {
      router.navigate('/timeline', { replace: true })
    } else if (path === '/reports' && !user.is_admin && !leads) {
      router.navigate('/timeline', { replace: true })
    } else if (path === '/login' || path === '/password' || (path === '/' && router.hosted)) {
      router.navigate('/timeline', { replace: true })
    }
  })

  async function logout() {
    await session.logout()
    router.navigate('/login', { replace: true })
  }
</script>

{#if selectingWorkspace}
  <WorkspaceSelect />
{:else if router.path === '/' && !router.base}
  <!-- Public homepage, also the hosted root: never wait on the session. -->
  <Page />
{:else}
  <header>
    <Link href={user ? '/timeline' : '/'} class="brand">Tstamps</Link>
    {#if user && !session.mustChangePassword}
      <nav>
        <Link href="/timeline">Timeline</Link>
        {#if user.is_admin || leads}
          <Link href="/reports">Reports</Link>
        {/if}
        {#if user.is_admin}
          <Link href="/admin/users">Admin</Link>
        {/if}
        <a href="/docs/">Docs</a>
      </nav>
      <div class="who">
        <span>{user.name} · {user.timezone}</span>
        <Button variant="secondary" onclick={logout}>Log out</Button>
      </div>
    {/if}
  </header>

  <main class:wide={router.path === '/timeline' || router.path === '/reports'}>
    {#if ready && allowed}
      <Page />
    {/if}
  </main>
{/if}
