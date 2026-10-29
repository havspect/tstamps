<script lang="ts">
  import { onMount } from 'svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { deviceTimezone, todayIn } from '../lib/format'
  import { monthLabel, nextMonth, periodLabel, planRange, previousMonth, weekLabel } from '../lib/plan'
  import type { PlanPeriod } from '../lib/plan'
  import { session } from '../lib/session.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Select from '../lib/ui/Select.svelte'

  type Board = components['schemas']['PlanBoardResponse']
  type Cell = components['schemas']['PlanCellResponse']
  type Person = components['schemas']['PlanPersonResponse']
  type Project = components['schemas']['ProjectResponse']
  type MemberRow = { userID: number; user: string; total: number; weeks: Record<string, number> }
  type ProjectRow = {
    projectID: number
    project: string
    teamID: number
    archived: boolean
    total: number
    weeks: Record<string, number>
    members: MemberRow[]
  }

  const user = session.user
  const isAdmin = user?.is_admin ?? false
  const isLead = user?.memberships?.some((member) => member.role === 'lead') ?? false
  const canEdit = isAdmin || isLead
  const zone = deviceTimezone()
  const today = todayIn(zone)

  let period = $state<PlanPeriod>('month')
  let day = $state(`${nextMonth(today)}-01`)
  let view = $state<'person' | 'project'>('person')
  let board = $state<Board | null>(null)
  let projects = $state<Project[]>([])
  let open = $state<number[]>([])
  let loading = $state(true)
  let error = $state('')
  let notice = $state('')

  let editing = $state<{ userID: number; projectID: number | null; week: string } | null>(null)
  let formProject = $state('')
  let formHours = $state('')

  const weeks = $derived(board?.weeks ?? [])
  const people = $derived(board?.people ?? [])
  const cells = $derived(board?.cells ?? [])
  const other = $derived(board?.other ?? [])
  const colors = $derived(new Map(projects.map((project) => [project.id, project.color])))
  const archived = $derived(new Set(cells.filter((cell) => cell.archived).map((cell) => cell.project_id)))
  const rangeLabel = $derived(periodLabel(period, day))

  const planned = $derived(cells.reduce((sum, cell) => sum + cell.hours, 0) + other.reduce((sum, row) => sum + row.hours, 0))
  const capacity = $derived(people.reduce((sum, person) => sum + person.weekly_hours * weeks.length, 0))

  const projectRows = $derived.by(() => {
    const rows = new Map<number, ProjectRow>()
    const rowOf = (id: number, name: string, teamID: number, isArchived: boolean) => {
      let row = rows.get(id)
      if (!row) {
        row = { projectID: id, project: name, teamID, archived: isArchived, total: 0, weeks: {}, members: [] }
        rows.set(id, row)
      }
      return row
    }
    for (const project of projects) {
      rowOf(project.id, project.name, project.team_id, Boolean(project.archived_at))
    }
    for (const cell of cells) {
      const row = rowOf(cell.project_id, cell.project, 0, cell.archived)
      row.weeks[cell.week_start] = (row.weeks[cell.week_start] ?? 0) + cell.hours
      row.total += cell.hours
    }

    const peopleByID = new Map(people.map((person) => [person.user_id, person]))
    for (const row of rows.values()) {
      const ids = new Set<number>()
      // Everyone in the project's team can be planned; an archived project only
      // keeps the people it already has hours for.
      if (row.teamID && !row.archived) {
        for (const person of people) {
          if ((person.team_ids ?? []).includes(row.teamID)) ids.add(person.user_id)
        }
      }
      for (const cell of cells) {
        if (cell.project_id === row.projectID) ids.add(cell.user_id)
      }
      row.members = [...ids]
        .map((userID) => {
          const member: MemberRow = { userID, user: peopleByID.get(userID)?.user ?? '', total: 0, weeks: {} }
          for (const cell of cells) {
            if (cell.project_id === row.projectID && cell.user_id === userID) {
              member.weeks[cell.week_start] = cell.hours
              member.total += cell.hours
            }
          }
          return member
        })
        .sort((a, b) => a.user.localeCompare(b.user))
    }
    return Array.from(rows.values()).sort((a, b) => b.total - a.total || a.project.localeCompare(b.project))
  })

  function cellsOf(personID: number, week: string): Cell[] {
    return cells.filter((cell) => cell.user_id === personID && cell.week_start === week)
  }

  function cellHours(personID: number, projectID: number, week: string): number {
    return cells.find((cell) => cell.user_id === personID && cell.project_id === projectID && cell.week_start === week)?.hours ?? 0
  }

  function personWeek(personID: number, week: string): number {
    const named = cellsOf(personID, week).reduce((sum, cell) => sum + cell.hours, 0)
    return named + otherOf(personID, week)
  }

  function personTotal(personID: number): number {
    const named = cells.filter((cell) => cell.user_id === personID).reduce((sum, cell) => sum + cell.hours, 0)
    const elsewhere = other.filter((row) => row.user_id === personID).reduce((sum, row) => sum + row.hours, 0)
    return named + elsewhere
  }

  function otherOf(personID: number, week: string): number {
    return other.find((row) => row.user_id === personID && row.week_start === week)?.hours ?? 0
  }

  function weeklyHours(personID: number): number {
    return people.find((person) => person.user_id === personID)?.weekly_hours ?? 0
  }

  function weekPlanned(week: string): number {
    const named = cells.filter((cell) => cell.week_start === week).reduce((sum, cell) => sum + cell.hours, 0)
    const elsewhere = other.filter((row) => row.week_start === week).reduce((sum, row) => sum + row.hours, 0)
    return named + elsewhere
  }

  function weekCapacity(week: string): number {
    return people.reduce((sum, person) => sum + person.weekly_hours, 0)
  }

  function over(person: Person): boolean {
    return person.weekly_hours > 0 && personTotal(person.user_id) > person.weekly_hours * weeks.length
  }

  function overWeek(personID: number, week: string): boolean {
    return weeklyHours(personID) > 0 && personWeek(personID, week) > weeklyHours(personID)
  }

  function toggle(projectID: number) {
    open = open.includes(projectID) ? open.filter((id) => id !== projectID) : [...open, projectID]
  }

  function choosePeriod(next: PlanPeriod) {
    if (period === next) return
    period = next
    void load()
  }

  async function load(keepEditor = false) {
    loading = true
    error = ''
    const { from, to } = planRange(period, day)
    const [planRes, projectRes] = await Promise.all([
      api.GET('/api/v1/plan', { params: { query: { from, to } } }),
      api.GET('/api/v1/projects', { params: { query: isAdmin ? { all: true } : {} } }),
    ])
    if (planRes.error) {
      error = problemMessage(planRes.error, planRes.response)
      board = null
    } else {
      board = planRes.data ?? null
    }
    projects = projectRes.data ?? []
    if (!keepEditor) editing = null
    loading = false
  }

  function select(personID: number, week: string, projectID: number | null = null) {
    if (!canEdit) return
    editing = { userID: personID, projectID, week }
    formProject = projectID ? String(projectID) : ''
    const existing = projectID ? cellHours(personID, projectID, week) : 0
    formHours = existing > 0 ? String(existing) : ''
    notice = ''
  }

  function currentHours(projectID: number): number {
    if (!editing) return 0
    return cellsOf(editing.userID, editing.week).find((cell) => cell.project_id === projectID)?.hours ?? 0
  }

  function chooseProject(id: string) {
    formProject = id
    // Fill the field with what is already planned, but never replace hours the
    // person already typed after choosing a project.
    if (id && formHours === '') formHours = String(currentHours(Number(id)))
  }

  async function save() {
    if (!editing) return
    const projectID = editing.projectID ?? Number(formProject)
    if (!projectID) {
      error = 'Choose a project first.'
      return
    }
    if (formHours.trim() === '') {
      error = 'Enter the hours, or 0 to clear this project from the week.'
      return
    }
    const hours = Number(formHours)
    if (!Number.isInteger(hours) || hours < 0 || hours > 168) {
      error = 'Hours must be a whole number from 0 to 168.'
      return
    }
    const { error: problem, response } = await api.PUT('/api/v1/plan', {
      body: {
        user_id: editing.userID,
        project_id: projectID,
        week_start: editing.week,
        hours,
      },
    })
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    error = ''
    notice = `Saved ${hours}h.`
    await load(true)
  }

  async function seed() {
    error = ''
    const month = day.slice(0, 7)
    const { data, error: problem, response } = await api.POST('/api/v1/plan/seed', {
      body: { from_month: previousMonth(month), into_month: month },
    })
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    notice = `Seeded ${data?.filled ?? 0} cells and skipped ${data?.skipped ?? 0}.`
    await load()
  }

  async function repeat() {
    if (!editing) return
    const mine = cellsOf(editing.userID, editing.week).filter(
      (cell) => !editing?.projectID || cell.project_id === editing.projectID,
    )
    if (mine.length === 0) {
      notice = 'Nothing is planned in this week, so there is nothing to repeat.'
      return
    }
    const { data, error: problem, response } = await api.POST('/api/v1/plan/repeat', {
      body: {
        week_start: editing.week,
        weeks: 4,
        user_id: editing.userID,
        ...(editing.projectID ? { project_id: editing.projectID } : {}),
      },
    })
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    error = ''
    const filled = data?.filled ?? 0
    const skipped = data?.skipped ?? 0
    const written = [...new Set((data?.cells ?? []).map((cell) => cell.week_start))]
    if (filled === 0) {
      notice = `Filled 0 cells, skipped ${skipped}: the following weeks are already planned.`
      await load(true)
      return
    }
    // If every filled week is outside the range on screen, show where the cells
    // landed; otherwise the button looks like it did nothing.
    const { from, to } = planRange(period, day)
    if (!written.some((week) => week >= from && week < to)) {
      day = written[0]
      notice = `Filled ${filled} cells, skipped ${skipped}. Showing ${periodLabel(period, day)}.`
      await load()
      return
    }
    const later = [...new Set(written.filter((week) => week < from || week >= to).map((week) => week.slice(0, 7)))]
      .map(monthLabel)
    notice = `Filled ${filled} cells, skipped ${skipped}.`
    if (later.length > 0) notice += ` Later weeks are in ${later.join(', ')}.`
    await load(true)
  }

  const editedPerson = $derived(people.find((person) => person.user_id === editing?.userID))
  const editedProject = $derived(
    editing?.projectID ? projectRows.find((row) => row.projectID === editing?.projectID)?.project : '',
  )

  onMount(() => {
    void load()
  })
</script>

<h1>Plan</h1>

<dl class="kpis" aria-label="Plan summary">
  <div>
    <dt>Planned</dt>
    <dd>{planned}h</dd>
  </div>
  <div>
    <dt>Capacity</dt>
    <dd>{capacity}h</dd>
  </div>
  <div>
    <dt>Unallocated</dt>
    <dd class:negative={capacity - planned < 0}>{capacity - planned}h</dd>
  </div>
  <div>
    <dt>People</dt>
    <dd>{people.length}</dd>
  </div>
</dl>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}
{#if notice && !editing}
  <p class="notice" role="status">{notice}</p>
{/if}

<form class="toolbar" onsubmit={(event) => { event.preventDefault(); void load() }}>
  <div class="field">
    <span class="field-label">View</span>
    <div class="tabs" role="group" aria-label="View">
      <Button variant="secondary" active={view === 'person'} onclick={() => (view = 'person')}>By person</Button>
      <Button variant="secondary" active={view === 'project'} onclick={() => (view = 'project')}>By project</Button>
    </div>
  </div>
  <div class="field">
    <span class="field-label">Range</span>
    <div class="tabs" role="group" aria-label="Range">
      <Button variant="secondary" active={period === 'week'} onclick={() => choosePeriod('week')}>Week</Button>
      <Button variant="secondary" active={period === 'month'} onclick={() => choosePeriod('month')}>Month</Button>
      <Button variant="secondary" active={period === 'quarter'} onclick={() => choosePeriod('quarter')}>Quarter</Button>
    </div>
  </div>
  <Input id="plan-day" label="Any day in range" type="date" bind:value={day} />
  <Button type="submit" disabled={loading}>Show</Button>
  {#if canEdit && view === 'person' && period === 'month'}
    <Button variant="secondary" onclick={() => void seed()}>Seed from last month</Button>
  {/if}
</form>

{#if loading}
  <p>Loading plan…</p>
{:else if view === 'person' && people.length === 0}
  <p>No people to plan in this range.</p>
{:else if view === 'person'}
  <wa-scroller class="board-scroller">
    <table class="board wa-tabular-nums">
      <caption class="range">{rangeLabel}</caption>
      <thead>
        <tr>
          <th scope="col">Person</th>
          {#each weeks as week (week)}
            <th scope="col" class="num" class:today={week === today}>Week of {weekLabel(week)}</th>
          {/each}
          <th scope="col" class="num">Total</th>
        </tr>
      </thead>
      <tbody>
        {#each people as person (person.user_id)}
          {@const total = personTotal(person.user_id)}
          <tr>
            <th scope="row">
              {person.user}
              {#if person.deactivated}<span class="teams">(deactivated)</span>{/if}
              {#if (person.teams ?? []).length > 0}<span class="teams">{(person.teams ?? []).join(', ')}</span>{/if}
            </th>
            {#each weeks as week (week)}
              {@const entries = cellsOf(person.user_id, week)}
              {@const plannedHours = entries.reduce((sum, cell) => sum + cell.hours, 0)}
              {@const elsewhere = otherOf(person.user_id, week)}
              <td class="num">
                <button
                  type="button"
                  class="cell"
                  class:over={person.weekly_hours > 0 && plannedHours + elsewhere > person.weekly_hours}
                  disabled={!canEdit}
                  aria-label={`${person.user}, week of ${weekLabel(week)}: ${plannedHours} of ${person.weekly_hours} hours planned`}
                  onclick={() => select(person.user_id, week)}
                >
                  <span class="bar">
                    {#each entries as cell (cell.project_id)}
                      <span
                        class="segment"
                        title={`${cell.project}: ${cell.hours}h`}
                        style:width={`${Math.min(100, (cell.hours / Math.max(person.weekly_hours, 1)) * 100)}%`}
                        style:background={colors.get(cell.project_id) ?? 'var(--muted)'}
                      ></span>
                    {/each}
                    {#if elsewhere > 0}
                      <span class="segment other" title={`Other teams: ${elsewhere}h`} style:width={`${Math.min(100, (elsewhere / Math.max(person.weekly_hours, 1)) * 100)}%`}></span>
                    {/if}
                  </span>
                  <span class="value">
                    {plannedHours}{#if elsewhere > 0}<span class="other-hours">+{elsewhere}</span>{/if}
                  </span>
                </button>
              </td>
            {/each}
            <td class="num total" class:over={over(person)}>
              {total}{person.weekly_hours > 0 ? ` / ${person.weekly_hours * weeks.length}h` : ''}
            </td>
          </tr>
        {/each}
      </tbody>
      <tfoot>
        <tr>
          <th scope="row">Unallocated</th>
          {#each weeks as week (week)}
            <td class="num">{weekCapacity(week) - weekPlanned(week)}h</td>
          {/each}
          <td class="num">{capacity - planned}h</td>
        </tr>
      </tfoot>
    </table>
  </wa-scroller>
{:else}
  <wa-scroller class="board-scroller">
    <table class="board wa-tabular-nums">
      <caption class="range">{rangeLabel}</caption>
      <thead>
        <tr>
          <th scope="col">Project</th>
          {#each weeks as week (week)}
            <th scope="col" class="num" class:today={week === today}>Week of {weekLabel(week)}</th>
          {/each}
          <th scope="col" class="num">Total</th>
        </tr>
      </thead>
      <tbody>
        {#each projectRows as row (row.projectID)}
          <tr>
            <th scope="row">
              <button
                type="button"
                class="project"
                aria-expanded={open.includes(row.projectID)}
                onclick={() => toggle(row.projectID)}
              >
                <span class="caret" aria-hidden="true">{open.includes(row.projectID) ? '▾' : '▸'}</span>
                <span class="swatch" style:background={colors.get(row.projectID) ?? 'var(--muted)'}></span>
                {row.project}
                {#if row.archived}<span class="badge">archived</span>{/if}
              </button>
            </th>
            {#each weeks as week (week)}
              <td class="num">{row.weeks[week] ?? ''}</td>
            {/each}
            <td class="num total">{row.total}h</td>
          </tr>
          {#if open.includes(row.projectID)}
            {#each row.members as member (member.userID)}
              <tr class="plan-member">
                <th scope="row">{member.user}</th>
                {#each weeks as week (week)}
                  <td class="num">
                    <button
                      type="button"
                      class="cell"
                      class:over={overWeek(member.userID, week)}
                      disabled={!canEdit || row.archived}
                      aria-label={`${member.user}, ${row.project}, week of ${weekLabel(week)}: ${member.weeks[week] ?? 0} hours planned`}
                      onclick={() => select(member.userID, week, row.projectID)}
                    >
                      <span class="value">{member.weeks[week] ?? ''}</span>
                    </button>
                  </td>
                {/each}
                <td class="num total">{member.total}h</td>
              </tr>
            {/each}
          {/if}
        {/each}
      </tbody>
    </table>
  </wa-scroller>
  {#if projectRows.length === 0}<p>Nothing planned in {rangeLabel}.</p>{/if}
{/if}

{#if editing && editedPerson}
  <section class="editor" aria-label="Edit plan cell">
    <h2>{editedPerson.user}{#if editedProject}{` · ${editedProject}`}{/if} · week of {weekLabel(editing.week)}</h2>
    <div class="editor-fields">
      {#if !editing.projectID}
        <Select id="plan-project" label="Project" bind:value={formProject} onchange={(event) => chooseProject(event.currentTarget.value)}>
          <wa-option value="">Choose a project</wa-option>
          {#each projects as project (project.id)}
            <wa-option value={String(project.id)}>{project.name}{archived.has(project.id) ? ' (archived)' : ''}</wa-option>
          {/each}
        </Select>
      {/if}
      <Input id="plan-hours" label="Hours per week" type="number" min="0" max="168" bind:value={formHours} />
      <Button onclick={() => void save()}>Set</Button>
      <Button variant="secondary" onclick={() => void repeat()}>Repeat 4 weeks</Button>
    </div>
    {#if !editing.projectID}
      <p class="hint">Set 0 to clear a project from this week.</p>
      <ul class="current">
        {#each cellsOf(editing.userID, editing.week) as cell (cell.project_id)}
          <li>{cell.project}: {cell.hours}h</li>
        {/each}
      </ul>
    {/if}
    {#if notice}<p class="notice" role="status">{notice}</p>{/if}
  </section>
{/if}

<style>
  .kpis {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    margin: 0 0 var(--space-3);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  .kpis > div {
    padding: var(--space-3);
    border-left: 1px solid var(--border);
  }

  .kpis > div:first-child {
    border-left: 0;
  }

  .kpis dt {
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--muted);
  }

  .kpis dd {
    margin: var(--space-1) 0 0;
    font-size: 1.5rem;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  }

  .kpis dd.negative {
    color: var(--danger, #b42318);
  }

  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }

  .field-label {
    display: block;
    margin-bottom: var(--space-1);
    font-size: 0.8125rem;
    color: var(--muted);
  }

  .tabs {
    display: flex;
    gap: var(--space-1);
  }

  .board-scroller {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }

  .board {
    width: 100%;
    border-collapse: collapse;
  }

  .range {
    padding: var(--space-2) var(--space-3);
    text-align: left;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--muted);
  }

  .board th,
  .board td {
    padding: var(--space-2);
    border-bottom: 1px solid var(--border);
    text-align: left;
    white-space: nowrap;
  }

  .board thead th {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
  }

  .board tfoot th,
  .board tfoot td {
    border-bottom: 0;
    color: var(--muted);
  }

  /* Every number sits centred in its row and against the right edge; names stay left. */
  .board th,
  .board td {
    vertical-align: middle;
  }

  .board tbody th,
  .board tfoot th {
    text-align: left;
  }

  .num {
    text-align: right;
  }

  /* The header and footer cells also carry .num; this outranks `.board th`. */
  .board .num {
    text-align: right;
  }

  .today {
    color: var(--accent, #4f46e5);
  }

  .teams {
    margin-left: var(--space-2);
    color: var(--muted);
    font-weight: 400;
    font-size: 0.8125rem;
  }

  .project {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .project:hover {
    color: var(--accent, #4f46e5);
  }

  .caret {
    width: 0.75rem;
    color: var(--muted);
    font-size: 0.75rem;
  }

  .plan-member th,
  .plan-member td {
    color: var(--muted);
  }

  .plan-member th:first-child {
    padding-left: var(--space-5);
    font-weight: 400;
  }

  .cell {
    display: inline-flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    width: 100%;
    padding: calc(var(--space-1) / 2);
    border: 0;
    border-radius: var(--radius);
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  .cell:disabled {
    cursor: default;
  }

  .bar {
    display: flex;
    flex: 1 1 auto;
    height: 0.5rem;
    min-width: 4rem;
    border-radius: 999px;
    background: var(--border);
    overflow: hidden;
  }

  .segment {
    display: block;
    height: 100%;
  }

  .segment.other {
    background: repeating-linear-gradient(45deg, var(--muted), var(--muted) 3px, transparent 3px, transparent 6px);
  }

  .cell.over {
    background: rgb(180 35 24 / 0.08);
  }

  .cell.over .bar {
    outline: 1px solid var(--danger, #b42318);
    outline-offset: 1px;
  }

  .cell.over .value {
    color: var(--danger, #b42318);
    font-weight: 650;
  }

  .value {
    min-width: 2rem;
    text-align: right;
  }

  .other-hours {
    margin-left: 0.25rem;
    color: var(--muted);
    font-size: 0.8125rem;
  }

  .total.over {
    color: var(--danger, #b42318);
    font-weight: 650;
  }

  .swatch {
    display: inline-block;
    width: 0.6rem;
    height: 0.6rem;
    margin-right: var(--space-1);
    border-radius: 2px;
  }

  .badge {
    margin-left: var(--space-1);
    padding: 0 0.25rem;
    border: 1px solid var(--border);
    border-radius: 3px;
    font-size: 0.75rem;
    color: var(--muted);
  }

  .editor {
    margin-top: var(--space-4);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }

  .editor h2 {
    margin: 0 0 var(--space-3);
    font-size: 1rem;
  }

  .editor-fields {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--space-3);
  }

  .current {
    margin: var(--space-3) 0 0;
    padding-left: 1.2rem;
    color: var(--muted);
  }

  .hint {
    margin: var(--space-2) 0 0;
    color: var(--muted);
    font-size: 0.8125rem;
  }

  .notice {
    color: var(--accent, #4f46e5);
  }
</style>
