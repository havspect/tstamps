<script lang="ts">
  import { onMount } from 'svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { deviceTimezone, formatDuration, todayIn } from '../lib/format'
  import { router } from '../lib/router.svelte'
  import {
    bucketRange,
    csvHref,
    pivot,
    planByProject,
    presetRange,
    summarize,
    weekStartOf,
    type BucketBy,
    type PlannedProject,
    type RangePreset,
    type ReportRow,
  } from '../lib/reports'
  import { session } from '../lib/session.svelte'
  import ReportTable from '../lib/ReportTable.svelte'
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

  let rows = $state<ReportRow[]>([])
  let planned = $state<Map<number, PlannedProject>>(new Map())
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
  const grouped = $derived(pivot(rows, planned))
  const summary = $derived(summarize(grouped))
  const today = todayIn(zone)
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
    // The plan API starts a range at the first Monday on or before it, and a
    // plan cell is one ISO week.
    const planQuery: { from: string; to: string; team_id?: number } = { from: weekStartOf(from), to }
    if (teamID) query.team_id = Number(teamID)
    if (query.team_id) planQuery.team_id = query.team_id

    const [reportRes, projectRes, planRes] = await Promise.all([
      api.GET('/api/v1/reports', { params: { query } }),
      api.GET('/api/v1/projects', { params: { query: isAdmin ? { all: true } : {} } }),
      by === 'day' ? Promise.resolve(null) : api.GET('/api/v1/plan', { params: { query: planQuery } }),
    ])
    if (reportRes.error) {
      error = problemMessage(reportRes.error, reportRes.response)
      rows = []
    } else {
      rows = reportRes.data?.rows ?? []
    }
    projects = projectRes.data ?? []
    // A failed plan read leaves the report readable, just without drift lines.
    planned = planRes && !planRes.error ? planByProject(planRes.data?.cells ?? [], buckets) : new Map()
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

  onMount(() => {
    void load()
    if (isAdmin) void loadTeams()
  })
</script>

<h1>Reports</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

{#if !loading || grouped.length > 0}
  <dl class="kpis" aria-label="Report summary">
    <div>
      <dt>Total logged</dt>
      <dd>{formatDuration(summary.total)}</dd>
    </div>
    <div>
      <dt>People</dt>
      <dd>{summary.people}<span class="kpi-note">with hours</span></dd>
    </div>
    <div>
      <dt>Average per person</dt>
      <dd>{formatDuration(summary.perPerson)}</dd>
    </div>
    <div>
      <dt>Busiest project</dt>
      <dd>
        {#if summary.busiest}
          <span class="kpi-project">
            {summary.busiest.project}{#if summary.busiest.archived}<span class="badge">archived</span>{/if}
          </span>
          <span class="kpi-note">{Math.round(summary.busiest.share * 100)}% of hours</span>
        {:else}
          <span class="kpi-empty">—</span>
        {/if}
      </dd>
    </div>
  </dl>
{/if}

<form class="toolbar" onsubmit={(event) => { event.preventDefault(); void load() }}>
  <div class="field">
    <span class="field-label">Group by</span>
    <div class="tabs" role="group" aria-label="Group by">
      <Button variant="secondary" active={by === 'day'} onclick={() => { by = 'day'; void load() }}>Day</Button>
      <Button variant="secondary" active={by === 'week'} onclick={() => { by = 'week'; void load() }}>Week</Button>
      <Button variant="secondary" active={by === 'month'} onclick={() => { by = 'month'; void load() }}>Month</Button>
    </div>
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
  <ReportTable projects={grouped} {buckets} {colors} {today} />
{/if}
