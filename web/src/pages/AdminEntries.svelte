<script lang="ts">
  import { onMount } from 'svelte'
  import AdminNav from '../lib/AdminNav.svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { currentWeek, deviceTimezone } from '../lib/format'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'

  type Entry = components['schemas']['EntryResponse']
  type Project = components['schemas']['ProjectResponse']

  const week = currentWeek(deviceTimezone())

  let from = $state(week.from)
  let to = $state(week.to)
  let entries = $state<Entry[]>([])
  let projects = $state<Project[]>([])
  let loading = $state(true)
  let error = $state('')
  let tab = $state<'live' | 'deleted'>('live')
  let restoring = $state<number | null>(null)

  const live = $derived(entries.filter((entry) => !entry.deleted_at))
  const deleted = $derived(entries.filter((entry) => entry.deleted_at))
  const shown = $derived(tab === 'live' ? live : deleted)

  function at(value: string, zone: string): string {
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'short', timeZone: zone }).format(
      new Date(value),
    )
  }

  function projectName(id: number): string {
    return projects.find((project) => project.id === id)?.name ?? `project ${id}`
  }

  async function load() {
    error = ''
    const [entryRes, projectRes] = await Promise.all([
      api.GET('/api/v1/entries', {
        params: { query: { include_deleted: true, from, to } },
      }),
      api.GET('/api/v1/projects', { params: { query: { all: true } } }),
    ])
    if (entryRes.error) error = problemMessage(entryRes.error, entryRes.response)
    else entries = entryRes.data ?? []
    if (projectRes.error) error = problemMessage(projectRes.error, projectRes.response)
    else projects = projectRes.data ?? []
    loading = false
  }

  onMount(() => {
    void load()
  })

  async function restore(entry: Entry) {
    error = ''
    restoring = entry.id
    try {
      const { error: problem, response } = await api.POST('/api/v1/entries/{id}/restore', {
        params: { path: { id: entry.id } },
      })
      if (problem) throw new Error(problemMessage(problem, response))
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      restoring = null
    }
  }
</script>

<AdminNav />
<h1>Deleted entries</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

<form class="toolbar" onsubmit={(event) => { event.preventDefault(); void load() }}>
  <Input id="entries-from" label="From" type="date" bind:value={from} required />
  <Input id="entries-to" label="To" type="date" bind:value={to} required />
  <Button type="submit" disabled={loading}>
    {loading ? 'Loading…' : 'Load'}
  </Button>
  <p class="hint">The end date is exclusive.</p>
</form>

<div class="tabs" role="group" aria-label="Entry state">
  <Button variant="secondary" active={tab === 'live'} onclick={() => (tab = 'live')}>
    Live ({live.length})
  </Button>
  <Button variant="secondary" active={tab === 'deleted'} onclick={() => (tab = 'deleted')}>
    Deleted ({deleted.length})
  </Button>
</div>

{#if loading}
  <p>Loading entries…</p>
{:else if shown.length === 0}
  <p>{tab === 'live' ? 'No live entries in this range.' : 'No deleted entries in this range.'}</p>
{:else}
  <wa-scroller>
    <table class="wa-hover-rows wa-tabular-nums">
      <thead>
        <tr>
          <th scope="col">ID</th>
          <th scope="col">User</th>
          <th scope="col">Project</th>
          <th scope="col">Start</th>
          <th scope="col">End</th>
          <th scope="col">Note</th>
          {#if tab === 'deleted'}
            <th scope="col">Deleted</th>
            <th scope="col">Actions</th>
          {/if}
        </tr>
      </thead>
      <tbody>
        {#each shown as entry (entry.id)}
          <tr>
            <td>{entry.id}</td>
            <td>{entry.user_id}</td>
            <td>{projectName(entry.project_id)}</td>
            <td>{at(entry.start_at, entry.timezone)}</td>
            <td>{entry.end_at ? at(entry.end_at, entry.timezone) : 'running'}</td>
            <td>{entry.note || '—'}</td>
            {#if tab === 'deleted'}
              <td>{entry.deleted_at ? at(entry.deleted_at, entry.timezone) : ''}</td>
              <td>
                <Button
                  variant="secondary"
                  onclick={() => restore(entry)}
                  disabled={restoring === entry.id}
                >
                  {restoring === entry.id ? 'Restoring…' : 'Restore'}
                </Button>
              </td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </wa-scroller>
{/if}
