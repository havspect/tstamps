<script lang="ts">
  import { onMount } from 'svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { deviceTimezone, todayIn } from '../lib/format'
  import { session } from '../lib/session.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Select from '../lib/ui/Select.svelte'

  type Vacation = components['schemas']['VacationResponse']
  type User = components['schemas']['UserResponse']
  type PersonOption = { id: number; name: string; email: string }

  const user = session.user
  const isAdmin = user?.is_admin ?? false
  const isLead = user?.memberships?.some((member) => member.role === 'lead') ?? false
  const canReadOthers = isAdmin || isLead
  const today = todayIn(deviceTimezone())
  const defaultDaily = user && user.weekly_hours > 0 ? user.weekly_hours / 5 : 8

  let vacations = $state<Vacation[]>([])
  let people = $state<PersonOption[]>([])
  let selectedUserID = $state(0)
  let loading = $state(true)
  let error = $state('')
  let notice = $state('')
  let saving = $state(false)
  let start = $state(today)
  let end = $state(today)
  let startHours = $state(formatHours(defaultDaily))
  let endHours = $state(formatHours(defaultDaily))
  let note = $state('')

  const singleDay = $derived(start === end)
  const startIsWeekend = $derived(isWeekend(start))
  const endIsWeekend = $derived(isWeekend(end))
  const selectedName = $derived(
    selectedUserID === 0 ? (user?.name ?? 'me') : (people.find((person) => person.id === selectedUserID)?.name ?? ''),
  )

  $effect(() => {
    if (startIsWeekend) startHours = '0'
    if (singleDay || endIsWeekend) endHours = '0'
    if (!startIsWeekend && startHours === '0') startHours = formatHours(defaultDaily)
    if (!singleDay && !endIsWeekend && endHours === '0') endHours = formatHours(defaultDaily)
  })

  function isWeekend(value: string): boolean {
    const day = new Date(`${value}T00:00:00Z`).getUTCDay()
    return day === 0 || day === 6
  }

  function formatHours(value: number): string {
    return Number(value.toFixed(1)).toString()
  }

  function hoursOf(value: string): number {
    return Number(value)
  }

  function validHours(value: string): boolean {
    const hours = hoursOf(value)
    return hours > 0 && hours <= defaultDaily + 1e-9 && Math.abs(hours * 10 - Math.round(hours * 10)) < 1e-6
  }

  async function loadPeople() {
    if (isAdmin) {
      const { data, error: problem, response } = await api.GET('/api/v1/users')
      if (problem) {
        error = problemMessage(problem, response)
        return
      }
      people = (data ?? []).filter((person: User) => person.id !== user?.id).map((person: User) => ({
        id: person.id, name: person.name, email: person.email,
      }))
      return
    }
    if (!isLead) return
    const led = new Set((user?.memberships ?? []).filter((member) => member.role === 'lead').map((member) => member.team_id))
    const { data, error: problem, response } = await api.GET('/api/v1/plan')
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    const found = new Map<number, PersonOption>()
    for (const person of data?.people ?? []) {
      if (person.user_id !== user?.id && (person.team_ids ?? []).some((teamID) => led.has(teamID))) {
        found.set(person.user_id, { id: person.user_id, name: person.user, email: person.email })
      }
    }
    people = [...found.values()].sort((a, b) => a.name.localeCompare(b.name))
  }

  async function load() {
    loading = true
    error = ''
    const query = selectedUserID === 0 ? {} : { user_id: selectedUserID }
    const { data, error: problem, response } = await api.GET('/api/v1/vacations', { params: { query } })
    if (problem) error = problemMessage(problem, response)
    else vacations = data ?? []
    loading = false
  }

  onMount(async () => {
    await load()
    await loadPeople()
  })

  function choosePerson(value: string) {
    selectedUserID = Number(value)
    notice = ''
    void load()
  }

  async function add(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    notice = ''
    if (end < start) {
      error = 'The last day cannot be before the first day.'
      return
    }
    if (!startIsWeekend && !validHours(startHours)) {
      error = `The first day needs more than 0 and at most ${formatHours(defaultDaily)} hours, in 0.1 steps.`
      return
    }
    if (!singleDay && !endIsWeekend && !validHours(endHours)) {
      error = `The last day needs more than 0 and at most ${formatHours(defaultDaily)} hours, in 0.1 steps.`
      return
    }
    saving = true
    const { error: problem, response } = await api.POST('/api/v1/vacations', {
      body: {
        start_date: start,
        end_date: end,
        start_hours: startIsWeekend ? 0 : hoursOf(startHours),
        end_hours: singleDay || endIsWeekend ? 0 : hoursOf(endHours),
        note,
      },
    })
    saving = false
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    note = ''
    notice = `Added ${start === end ? start : `${start} to ${end}`}.`
    await load()
  }

  async function remove(vacation: Vacation) {
    const range = vacation.start_date === vacation.end_date
      ? vacation.start_date
      : `${vacation.start_date} to ${vacation.end_date}`
    if (!confirm(`Delete the vacation ${range}?`)) return
    error = ''
    notice = ''
    const { error: problem, response } = await api.DELETE('/api/v1/vacations/{id}', {
      params: { path: { id: vacation.id } },
    })
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    notice = `Deleted ${range}.`
    await load()
  }

  function hoursLabel(vacation: Vacation): string {
    if (vacation.start_date === vacation.end_date) return `${vacation.start_hours}h`
    return `${vacation.start_hours}h first · ${vacation.end_hours}h last`
  }
</script>

<h1>Vacations</h1>

<p class="hint">
  Time off lowers the capacity the plan board shows for you. It is not logged time, so it never appears in
  reports or on the timeline.
</p>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}
{#if notice}
  <p class="notice" role="status">{notice}</p>
{/if}

{#if canReadOthers && people.length > 0}
  <div class="toolbar">
    <Select id="vacation-person" label="Person" value={String(selectedUserID)} onchange={(event) => choosePerson(event.currentTarget.value)}>
      <wa-option value="0">Me ({user?.name})</wa-option>
      {#each people as person (person.id)}
        <wa-option value={String(person.id)}>{person.name}</wa-option>
      {/each}
    </Select>
  </div>
{/if}

{#if selectedUserID === 0}
  <form class="toolbar" onsubmit={add}>
    <Input id="vacation-start" label="First day" type="date" bind:value={start} required />
    <Input id="vacation-end" label="Last day" type="date" bind:value={end} required />
    {#if singleDay}
      <Input
        id="vacation-start-hours"
        label="Hours off"
        type="number"
        min="0"
        max={formatHours(defaultDaily)}
        step="0.1"
        bind:value={startHours}
        disabled={startIsWeekend}
      />
    {:else}
      <Input
        id="vacation-start-hours"
        label="First day hours"
        type="number"
        min="0"
        max={formatHours(defaultDaily)}
        step="0.1"
        bind:value={startHours}
        disabled={startIsWeekend}
      />
      <Input
        id="vacation-end-hours"
        label="Last day hours"
        type="number"
        min="0"
        max={formatHours(defaultDaily)}
        step="0.1"
        bind:value={endHours}
        disabled={endIsWeekend}
      />
    {/if}
    <Input id="vacation-note" label="Note" bind:value={note} placeholder="Optional" />
    <Button type="submit" disabled={saving}>{saving ? 'Adding…' : 'Add vacation'}</Button>
  </form>
  {#if (startIsWeekend || (!singleDay && endIsWeekend))}
    <p class="hint">Weekends never count, so their hours are fixed at 0.</p>
  {/if}
{:else}
  <p class="hint">Read-only: {selectedName}'s vacations. Only the owner adds or deletes time off.</p>
{/if}

{#if loading}
  <p>Loading vacations…</p>
{:else if vacations.length === 0}
  <p>No vacations recorded for {selectedUserID === 0 ? 'you' : selectedName}.</p>
{:else}
  <wa-scroller>
    <table class="wa-hover-rows wa-tabular-nums">
      <caption class="hint">Newest first.</caption>
      <thead>
        <tr>
          {#if canReadOthers}
            <th scope="col">Person</th>
          {/if}
          <th scope="col">First day</th>
          <th scope="col">Last day</th>
          <th scope="col">Time off</th>
          <th scope="col">Note</th>
          {#if selectedUserID === 0}
            <th scope="col">Actions</th>
          {/if}
        </tr>
      </thead>
      <tbody>
        {#each vacations as vacation (vacation.id)}
          <tr>
            {#if canReadOthers}
              <td>{vacation.user}</td>
            {/if}
            <td>{vacation.start_date}</td>
            <td>{vacation.end_date}</td>
            <td>{hoursLabel(vacation)}</td>
            <td>{vacation.note}</td>
            {#if selectedUserID === 0}
              <td><Button variant="danger" onclick={() => void remove(vacation)}>Delete</Button></td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </wa-scroller>
{/if}
