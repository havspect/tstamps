<script lang="ts">
  import { onMount } from 'svelte'
  import AdminNav from '../lib/AdminNav.svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'

  type Team = components['schemas']['TeamResponse']
  type User = components['schemas']['UserResponse']
  type TeamMemberInput = components['schemas']['TeamMemberInput']
  type Role = TeamMemberInput['role']

  let teams = $state<Team[]>([])
  let users = $state<User[]>([])
  let loading = $state(true)
  let error = $state('')

  let teamName = $state('')
  let createPending = $state(false)

  let renaming = $state<number | null>(null)
  let renameName = $state('')
  let renamePending = $state(false)

  let archiving = $state<number | null>(null)

  let expanded = $state<number | null>(null)
  let drafts = $state<Record<number, Record<number, Role>>>({})
  let membersPending = $state<number | null>(null)

  async function load() {
    const { data, error: problem, response } = await api.GET('/api/v1/teams')
    if (problem) error = problemMessage(problem, response)
    else teams = data ?? []
    loading = false
  }

  async function loadUsers() {
    const { data, error: problem, response } = await api.GET('/api/v1/users')
    if (problem) error = problemMessage(problem, response)
    else users = data ?? []
  }

  onMount(() => {
    void load()
    void loadUsers()
  })

  async function create(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    createPending = true
    try {
      const { error: problem, response } = await api.POST('/api/v1/teams', { body: { name: teamName } })
      if (problem) throw new Error(problemMessage(problem, response))
      teamName = ''
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      createPending = false
    }
  }

  function startRename(team: Team) {
    if (renaming === team.id) {
      renaming = null
      return
    }
    renaming = team.id
    renameName = team.name
  }

  async function rename(event: SubmitEvent, team: Team) {
    event.preventDefault()
    error = ''
    renamePending = true
    try {
      const { error: problem, response } = await api.PATCH('/api/v1/teams/{id}', {
        params: { path: { id: team.id } },
        body: { name: renameName },
      })
      if (problem) throw new Error(problemMessage(problem, response))
      renaming = null
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      renamePending = false
    }
  }

  async function archive(team: Team) {
    if (
      !confirm(
        `Archive ${team.name}? This also archives the team's projects; their hours are kept. There is no restore.`,
      )
    )
      return
    error = ''
    archiving = team.id
    try {
      const { error: problem, response } = await api.DELETE('/api/v1/teams/{id}', {
        params: { path: { id: team.id } },
      })
      if (problem) throw new Error(problemMessage(problem, response))
      if (expanded === team.id) expanded = null
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      archiving = null
    }
  }

  function resetDraft(team: Team) {
    const draft: Record<number, Role> = {}
    for (const member of team.members ?? []) draft[member.user_id] = member.role === 'lead' ? 'lead' : 'member'
    drafts[team.id] = draft
  }

  function toggleMembers(team: Team) {
    if (expanded === team.id) {
      expanded = null
      return
    }
    expanded = team.id
    resetDraft(team)
  }

  function toggleMember(teamID: number, userID: number, checked: boolean) {
    if (checked) drafts[teamID][userID] = 'member'
    else delete drafts[teamID][userID]
  }

  function setRole(teamID: number, userID: number, role: string) {
    drafts[teamID][userID] = role === 'lead' ? 'lead' : 'member'
  }

  async function saveMembers(event: SubmitEvent, team: Team) {
    event.preventDefault()
    error = ''
    membersPending = team.id
    try {
      const draft = drafts[team.id] ?? {}
      const body: TeamMemberInput[] = Object.entries(draft).map(([id, role]) => ({
        user_id: Number(id),
        role,
      }))
      const { error: problem, response } = await api.PUT('/api/v1/teams/{id}/members', {
        params: { path: { id: team.id } },
        body,
      })
      if (problem) throw new Error(problemMessage(problem, response))
      await load()
      const fresh = teams.find((t) => t.id === team.id)
      if (fresh) resetDraft(fresh)
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      membersPending = null
    }
  }
</script>

<AdminNav />
<h1>Teams</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

<form class="toolbar" onsubmit={create}>
  <Input id="new-team" label="Name" bind:value={teamName} required />
  <Button type="submit" disabled={createPending}>
    {createPending ? 'Creating…' : 'Create team'}
  </Button>
</form>

{#if loading}
  <p>Loading teams…</p>
{:else if teams.length === 0}
  <p>No teams yet.</p>
{:else}
  <wa-scroller>
    <table class="wa-hover-rows wa-tabular-nums">
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Members</th>
          <th scope="col">Status</th>
          <th scope="col">Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each teams as team (team.id)}
          <tr>
            <td>{team.name}</td>
            <td>{team.members?.length ?? 0}</td>
            <td>
              {#if team.archived_at}
                <span class="badge alert">archived</span>
              {/if}
            </td>
            <td>
              <div class="row-actions">
                <Button variant="secondary" onclick={() => startRename(team)}>
                  {renaming === team.id ? 'Close' : 'Rename'}
                </Button>
                <Button variant="secondary" onclick={() => toggleMembers(team)}>
                  {expanded === team.id ? 'Close members' : 'Members'}
                </Button>
                <Button
                  variant="secondary"
                  onclick={() => archive(team)}
                  disabled={archiving === team.id || team.archived_at !== null}
                >
                  {archiving === team.id ? 'Archiving…' : 'Archive'}
                </Button>
              </div>
            </td>
          </tr>
          {#if renaming === team.id}
            <tr>
              <td colspan="4">
                <form class="toolbar" onsubmit={(event) => rename(event, team)}>
                  <Input id={`rename-${team.id}`} label="Name" bind:value={renameName} required />
                  <Button type="submit" disabled={renamePending}>
                    {renamePending ? 'Saving…' : 'Save'}
                  </Button>
                  <Button variant="secondary" onclick={() => (renaming = null)}>Cancel</Button>
                </form>
              </td>
            </tr>
          {/if}
          {#if expanded === team.id}
            <tr>
              <td colspan="4" class="members-cell">
                <form onsubmit={(event) => saveMembers(event, team)}>
                  <div class="member-list">
                    {#each users as user (user.id)}
                      <div class="member">
                        <wa-checkbox
                          checked={drafts[team.id]?.[user.id] !== undefined}
                          onchange={(event) =>
                            toggleMember(team.id, user.id, event.currentTarget.checked)}
                        >
                          {user.name} <span class="hint">{user.email}</span>
                        </wa-checkbox>
                        <select
                          aria-label={`Role for ${user.name}`}
                          value={drafts[team.id]?.[user.id] ?? 'member'}
                          onchange={(event) => setRole(team.id, user.id, event.currentTarget.value)}
                          disabled={drafts[team.id]?.[user.id] === undefined}
                        >
                          <option value="member">member</option>
                          <option value="lead">lead</option>
                        </select>
                      </div>
                    {/each}
                  </div>
                  <div class="row-actions">
                    <Button type="submit" disabled={membersPending === team.id}>
                      {membersPending === team.id ? 'Saving…' : 'Save members'}
                    </Button>
                    <Button variant="secondary" onclick={() => (expanded = null)}>Cancel</Button>
                    <p class="hint">Saving replaces the whole member list.</p>
                  </div>
                </form>
              </td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
  </wa-scroller>
{/if}
