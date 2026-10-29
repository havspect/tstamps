<script lang="ts">
  import { onMount } from 'svelte'
  import AdminNav from '../lib/AdminNav.svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Select from '../lib/ui/Select.svelte'
  import { PROJECT_COLORS } from '../lib/colors'

  type Project = components['schemas']['ProjectResponse']
  type Team = components['schemas']['TeamResponse']

  const defaultColor = PROJECT_COLORS[0]

  let projects = $state<Project[]>([])
  let teams = $state<Team[]>([])
  let loading = $state(true)
  let error = $state('')

  let name = $state('')
  let teamID = $state('')
  let color = $state(defaultColor)
  let createPending = $state(false)

  let editing = $state<number | null>(null)
  let editName = $state('')
  let editColor = $state(defaultColor)
  let editTeam = $state('')
  let savePending = $state(false)
  let archiving = $state<number | null>(null)
  let restoring = $state<number | null>(null)

  const activeTeams = $derived(teams.filter((team) => team.archived_at === null))

  function teamName(id: number): string {
    return teams.find((team) => team.id === id)?.name ?? `team ${id}`
  }

  function teamArchived(id: number): boolean {
    return teams.find((team) => team.id === id)?.archived_at !== null
  }

  async function load() {
    const [projectRes, teamRes] = await Promise.all([
      api.GET('/api/v1/projects', { params: { query: { all: true } } }),
      api.GET('/api/v1/teams'),
    ])
    if (projectRes.error) error = problemMessage(projectRes.error, projectRes.response)
    else projects = projectRes.data ?? []
    if (teamRes.error) error = problemMessage(teamRes.error, teamRes.response)
    else teams = teamRes.data ?? []
    loading = false
  }

  onMount(() => {
    void load()
  })

  async function create(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    createPending = true
    try {
      const { error: problem, response } = await api.POST('/api/v1/projects', {
        body: { name, team_id: Number(teamID), color },
      })
      if (problem) throw new Error(problemMessage(problem, response))
      name = ''
      color = defaultColor
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      createPending = false
    }
  }

  function startEdit(project: Project) {
    if (editing === project.id) {
      editing = null
      return
    }
    editing = project.id
    editName = project.name
    editColor = project.color || defaultColor
    editTeam = String(project.team_id)
  }

  async function save(event: SubmitEvent, project: Project) {
    event.preventDefault()
    error = ''
    savePending = true
    try {
      const { error: problem, response } = await api.PATCH('/api/v1/projects/{id}', {
        params: { path: { id: project.id } },
        body: { name: editName, color: editColor, team_id: Number(editTeam) },
      })
      if (problem) throw new Error(problemMessage(problem, response))
      editing = null
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      savePending = false
    }
  }

  async function archive(project: Project) {
    if (!confirm(`Archive ${project.name}? Its hours stay in reports, and you can restore it later.`)) return
    error = ''
    archiving = project.id
    try {
      const { error: problem, response } = await api.PATCH('/api/v1/projects/{id}', {
        params: { path: { id: project.id } },
        body: { archived: true },
      })
      if (problem) throw new Error(problemMessage(problem, response))
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      archiving = null
    }
  }

  async function restore(project: Project) {
    error = ''
    restoring = project.id
    try {
      const { error: problem, response } = await api.POST('/api/v1/projects/{id}/restore', {
        params: { path: { id: project.id } },
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

{#snippet palette(value: string, pick: (color: string) => void)}
  <div class="field">
    <span class="field-label">Color</span>
    <div class="swatches" role="group" aria-label="Project color">
      {#each PROJECT_COLORS as swatch (swatch)}
        <button
          type="button"
          class="swatch-button"
          style:background={swatch}
          aria-label={swatch}
          aria-pressed={value === swatch}
          onclick={() => pick(swatch)}
        ></button>
      {/each}
    </div>
  </div>
{/snippet}

<AdminNav />
<h1>Projects</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

<form class="toolbar" onsubmit={create}>
  <Input id="new-project" label="Name" bind:value={name} required />
  <Select id="new-project-team" label="Team" bind:value={teamID} required>
    <wa-option value="">Choose a team</wa-option>
    {#each activeTeams as team (team.id)}
      <wa-option value={String(team.id)}>{team.name}</wa-option>
    {/each}
  </Select>
  {@render palette(color, (c) => (color = c))}
  <Button type="submit" disabled={createPending || activeTeams.length === 0}>
    {createPending ? 'Creating…' : 'Create project'}
  </Button>
  {#if activeTeams.length === 0}
    <p class="hint">Create an active team first.</p>
  {/if}
</form>

{#if loading}
  <p>Loading projects…</p>
{:else if projects.length === 0}
  <p>No projects yet.</p>
{:else}
  <wa-scroller>
    <table class="wa-hover-rows">
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Team</th>
          <th scope="col">Status</th>
          <th scope="col">Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each projects as project (project.id)}
          <tr>
            <td>
              <span class="swatch" style={`background: ${project.color || 'transparent'}`} aria-hidden="true"
              ></span>
              {project.name}
            </td>
            <td>{teamName(project.team_id)}</td>
            <td>
              {#if project.archived_at}
                <span class="badge alert">archived</span>
              {/if}
            </td>
            <td>
              <div class="row-actions">
                <Button variant="secondary" onclick={() => startEdit(project)}>
                  {editing === project.id ? 'Close' : 'Edit'}
                </Button>
                {#if project.archived_at}
                  <Button
                    variant="secondary"
                    onclick={() => restore(project)}
                    disabled={restoring === project.id || teamArchived(project.team_id)}
                  >
                    {restoring === project.id ? 'Restoring…' : 'Restore'}
                  </Button>
                  {#if teamArchived(project.team_id)}
                    <span class="hint">Team archived</span>
                  {/if}
                {:else}
                  <Button
                    variant="secondary"
                    onclick={() => archive(project)}
                    disabled={archiving === project.id}
                  >
                    {archiving === project.id ? 'Archiving…' : 'Archive'}
                  </Button>
                {/if}
              </div>
            </td>
          </tr>
          {#if editing === project.id}
            <tr>
              <td colspan="4">
                <form class="toolbar" onsubmit={(event) => save(event, project)}>
                  <Input
                    id={`project-name-${project.id}`}
                    label="Name"
                    bind:value={editName}
                    required
                  />
                  <Select
                    id={`project-team-${project.id}`}
                    label="Team"
                    bind:value={editTeam}
                    hint="A project can only change team while it has no entries."
                  >
                    {#each teams as team (team.id)}
                      <wa-option value={String(team.id)}>
                        {team.name}{team.archived_at ? ' (archived)' : ''}
                      </wa-option>
                    {/each}
                  </Select>
                  {@render palette(editColor, (c) => (editColor = c))}
                  <Button type="submit" disabled={savePending}>
                    {savePending ? 'Saving…' : 'Save'}
                  </Button>
                  <Button variant="secondary" onclick={() => (editing = null)}>Cancel</Button>
                </form>
              </td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
  </wa-scroller>
{/if}
