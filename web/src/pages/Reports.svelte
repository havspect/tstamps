<script lang="ts">
  import { onMount } from 'svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { deviceTimezone, formatDuration } from '../lib/format'
  import { router } from '../lib/router.svelte'
  import {
    bucketRange,
    csvHref,
    pivot,
    presetRange,
    type BucketBy,
    type RangePreset,
    type ReportRow,
  } from '../lib/reports'
  import { session } from '../lib/session.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Select from '../lib/ui/Select.svelte'

  type Project = components['schemas']['ProjectResponse']
  type Team = components['schemas']['TeamResponse']

  const user = session.user
  const isAdmin = user?.is_admin ?? false
  const zone = deviceTimezone()
  const start = presetRange('this-week', zone)

  let preset = $state<RangePreset>('this-week')
  let from = $state(start.from)
  let to = $state(start.to)
  let by = $state<BucketBy>(start.by)
  let teamID = $state('')
  let open = $state<number[]>([])

  let rows = $state<ReportRow[]>([])
  let projects = $state<Project[]>([])
  let adminTeams = $state<Team[]>([])
  let loading = $state(true)
  let error = $state('')

  const teams = $derived(
    isAdmin
      ? adminTeams.map((team) => ({ id: team.id, name: team.name, archived: team.archived_at !== null }))
      : (user?.memberships ?? [])
          .filter((member) => member.role === 'lead')
          .map((member) => ({ id: member.team_id, name: member.team, archived: false })),
  )
  const buckets = $derived(bucketRange(from, to, by))
  const grouped = $derived(pivot(rows))
  const columns = $derived(
    buckets.map((bucket) => grouped.reduce((sum, project) => sum + (project.cells[bucket.key] ?? 0), 0)),
  )
  const grand = $derived(grouped.reduce((sum, project) => sum + project.total, 0))
  // Project colors are the timeline's, so the same project reads the same in both views.
  const colors = $derived(new Map(projects.map((project) => [project.id, project.color])))
  const csv = $derived(csvHref({ from, to, teamID }, router.base))
  const filename = $derived(`reports-${from}-to-${to}.csv`)

  function rangeError(left: string, right: string): string {
    const f = Date.parse(`${left}T00:00:00Z`)
    const t = Date.parse(`${right}T00:00:00Z`)
    if (Number.isNaN(f) || Number.isNaN(t)) return 'Choose a valid start and end date.'
    if (t <= f) return 'The end date must be after the start date; the end is exclusive.'
    if (t - f > 366 * 86_400_000) return 'Choose a range of at most 366 days.'
    return ''
  }

  async function loadTeams() {
    const { data, error: problem, response } = await api.GET('/api/v1/teams')
    if (problem) error = problemMessage(problem, response)
    else adminTeams = data ?? []
  }

  async function load() {
    error = rangeError(from, to)
    if (error) return
    loading = true
    const query: { from: string; to: string; by: BucketBy; team_id?: number } = { from, to, by }
    if (teamID) query.team_id = Number(teamID)

    const [reportRes, projectRes] = await Promise.all([
      api.GET('/api/v1/reports', { params: { query } }),
      api.GET('/api/v1/projects', { params: { query: isAdmin ? { all: true } : {} } }),
    ])
    if (reportRes.error) {
      error = problemMessage(reportRes.error, reportRes.response)
      rows = []
    } else {
      rows = reportRes.data?.rows ?? []
    }
    projects = projectRes.data ?? []
    // A person row under a project that left the range would keep it open for nothing.
    open = []
    loading = false
  }

  function choosePreset(value: string) {
    preset = value as RangePreset
    if (preset === 'custom') return
    const range = presetRange(preset, zone)
    from = range.from
    to = range.to
    by = range.by
    void load()
  }

  function toggle(projectID: number) {
    open = open.includes(projectID) ? open.filter((id) => id !== projectID) : [...open, projectID]
  }

  const cell = (seconds: number | undefined) => (seconds ? formatDuration(seconds) : '')

  onMount(() => {
    void load()
    if (isAdmin) void loadTeams()
  })
</script>

<h1>Reports</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

<form class="toolbar" onsubmit={(event) => { event.preventDefault(); void load() }}>
  <div class="tabs" role="group" aria-label="Group by">
    <Button variant="secondary" active={by === 'day'} onclick={() => { by = 'day'; void load() }}>Day</Button>
    <Button variant="secondary" active={by === 'week'} onclick={() => { by = 'week'; void load() }}>Week</Button>
    <Button variant="secondary" active={by === 'month'} onclick={() => { by = 'month'; void load() }}>Month</Button>
  </div>
  <Select
    id="report-range"
    label="Range"
    bind:value={preset}
    onchange={(event) => choosePreset(event.currentTarget.value)}
  >
    <wa-option value="this-week">This week</wa-option>
    <wa-option value="last-week">Last week</wa-option>
    <wa-option value="this-month">This month</wa-option>
    <wa-option value="last-month">Last month</wa-option>
    <wa-option value="this-year">This year</wa-option>
    <wa-option value="custom">Custom…</wa-option>
  </Select>
  {#if preset === 'custom'}
    <Input id="report-from" label="From" type="date" bind:value={from} required />
    <Input id="report-to" label="To" type="date" bind:value={to} required />
    <Button type="submit" disabled={loading}>Apply</Button>
  {/if}
  {#if teams.length > 1}
    <Select
      id="report-team"
      label="Team"
      bind:value={teamID}
      onchange={(event) => { teamID = event.currentTarget.value; void load() }}
    >
      <wa-option value="">All teams</wa-option>
      {#each teams as team (team.id)}
        <wa-option value={String(team.id)}>{team.name}{team.archived ? ' (archived)' : ''}</wa-option>
      {/each}
    </Select>
  {/if}
  <a class="csv" href={csv} download={filename}>Download CSV</a>
</form>

{#if loading}
  <p>Loading report…</p>
{:else if grouped.length === 0}
  <p>No hours in this range.</p>
{:else}
  <table class="report">
    <thead>
      <tr>
        <th scope="col">Project</th>
        {#each buckets as bucket (bucket.key)}
          <th scope="col" class="num">{bucket.label}</th>
        {/each}
        <th scope="col" class="num">Total</th>
      </tr>
    </thead>
    <tbody>
      {#each grouped as project (project.project_id)}
        <tr>
          <th scope="row">
            <button
              type="button"
              class="project"
              aria-expanded={open.includes(project.project_id)}
              onclick={() => toggle(project.project_id)}
            >
              <span class="caret" aria-hidden="true">{open.includes(project.project_id) ? '▾' : '▸'}</span>
              <span class="swatch" style:background={colors.get(project.project_id) ?? 'var(--muted)'}></span>
              {project.project}
              {#if project.archived}<span class="badge">archived</span>{/if}
            </button>
          </th>
          {#each buckets as bucket (bucket.key)}
            <td class="num">{cell(project.cells[bucket.key])}</td>
          {/each}
          <td class="num">{formatDuration(project.total)}</td>
        </tr>
        {#if open.includes(project.project_id)}
          {#each project.people as person (person.user_id)}
            <tr class="person">
              <td>{person.user}</td>
              {#each buckets as bucket (bucket.key)}
                <td class="num">{cell(person.cells[bucket.key])}</td>
              {/each}
              <td class="num">{formatDuration(person.total)}</td>
            </tr>
          {/each}
        {/if}
      {/each}
    </tbody>
    <tfoot>
      <tr>
        <th scope="row">Total</th>
        {#each columns as seconds, index (buckets[index].key)}
          <td class="num">{cell(seconds)}</td>
        {/each}
        <td class="num">{formatDuration(grand)}</td>
      </tr>
    </tfoot>
  </table>
{/if}

<style>
  .csv {
    align-self: end;
    margin-left: auto;
    padding-bottom: var(--space-2);
  }

  .report .num {
    text-align: right;
    white-space: nowrap;
  }

  .report tfoot th,
  .report tfoot td {
    border-top: 2px solid var(--text);
    border-bottom: 0;
    font-weight: 650;
  }

  .report tbody th {
    font-weight: inherit;
    text-transform: none;
    letter-spacing: 0;
    font-size: inherit;
    color: inherit;
  }

  .project {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) 0;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
  }

  .caret {
    width: 0.75rem;
    color: var(--muted);
    font-size: 0.75rem;
  }

  .project:hover {
    color: var(--accent);
  }

  .person td {
    color: var(--muted);
  }

  .person td:first-child {
    padding-left: var(--space-5);
  }
</style>
