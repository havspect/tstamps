<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { deviceTimezone, formatDuration } from '../lib/format'
  import { session } from '../lib/session.svelte'
  import { FINE_SNAP, SNAP, clampRange, orderProjects, snap } from '../lib/timeline/layout'
  import { addDays, instantToWallTime, localDate, wallTimeToInstant, zonedParts } from '../lib/timeline/time'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Select from '../lib/ui/Select.svelte'
  import Textarea from '../lib/ui/Textarea.svelte'

  type Entry = components['schemas']['EntryResponse']
  type Project = components['schemas']['ProjectResponse']

  const MIN = 60_000
  const HOUR = 3_600_000
  // One day is always 24 * HOUR_H tall: positions stay linear in epoch ms, so a
  // 23- or 25-hour DST day is simply scaled, not lied about.
  const HOUR_H = 56
  const AXIS_W = 64
  const LANE_W = 232
  const ADD_W = 96
  const HEAD_H = 44
  const LANES_KEY = 'tstamps.timeline.lanes'

  const user = session.user
  const userID = user?.id ?? 0
  // The axis follows the device, so typing 09:00 puts the block at 09:00.
  const tz = deviceTimezone()

  const clock = new Intl.DateTimeFormat(undefined, { timeZone: tz, hour: '2-digit', minute: '2-digit' })
  const formatters = new Map<string, { clock: Intl.DateTimeFormat; stamp: Intl.DateTimeFormat }>()
  function formattersFor(zone: string) {
    let f = formatters.get(zone)
    if (!f) {
      f = {
        clock: new Intl.DateTimeFormat(undefined, { timeZone: zone, hour: '2-digit', minute: '2-digit' }),
        stamp: new Intl.DateTimeFormat(undefined, { timeZone: zone, dateStyle: 'medium', timeStyle: 'short' }),
      }
      formatters.set(zone, f)
    }
    return f
  }
  const dayLabel = new Intl.DateTimeFormat(undefined, {
    timeZone: 'UTC',
    weekday: 'short',
    day: 'numeric',
    month: 'short',
  })

  let now = $state(Date.now())
  let day = $state(localDate(tz, Date.now()))
  let entries = $state<Entry[]>([])
  let projects = $state<Project[]>([])
  // Lane order is a preference, not data: projects that have entries always
  // appear even when they are not remembered here.
  let laneIDs = $state<number[]>([])
  let loading = $state(true)
  let pending = $state(false)
  let error = $state('')
  let lastProject = $state(0)
  let selectedID = $state<number | null>(null)

  let scrollEl = $state<HTMLDivElement>()
  let bodyEl = $state<HTMLDivElement>()
  let pickerOpen = $state(false)
  let pickerFilter = $state('')

  const dayStart = $derived(wallTimeToInstant(tz, `${day}T00:00`))
  const dayEnd = $derived(wallTimeToInstant(tz, `${addDays(day, 1)}T00:00`))
  const msPerPx = $derived((dayEnd - dayStart) / (24 * HOUR_H))
  const today = $derived(localDate(tz, now))
  const dayTitle = $derived(dayLabel.format(new Date(`${day}T00:00:00Z`)))
  const bodyHeight = 24 * HOUR_H

  function yOf(ms: number): number {
    return (ms - dayStart) / msPerPx
  }

  const projectByID = $derived(new Map(projects.map((project) => [project.id, project])))
  const choices = $derived(projects.filter((project) => !project.archived_at))
  const running = $derived(entries.find((entry) => entry.end_at === null) ?? null)

  function nameOf(entry: Entry): string {
    return projectByID.get(entry.project_id)?.name ?? `Archived project #${entry.project_id}`
  }

  function describe(entry: Entry, start: number, end: number): string {
    const zone = entry.timezone || tz
    const f = formattersFor(zone)
    const note = entry.note ? `, ${entry.note}` : ''
    const elsewhere = zone === tz ? '' : ` (${zone})`
    return `${nameOf(entry)}, ${f.stamp.format(start)} to ${f.clock.format(end)}${elsewhere} (${formatDuration(
      (end - start) / 1000,
    )})${note}`
  }

  type Drag =
    | {
        mode: 'create'
        projectID: number
        anchor: number
        start: number
        end: number
        originY: number
        moved: boolean
        pointerID: number
      }
    | {
        mode: 'move' | 'resize-start' | 'resize-end'
        id: number
        origStart: number
        origEnd: number
        start: number
        end: number
        originY: number
        moved: boolean
        running: boolean
        pointerID: number
      }

  let drag = $state<Drag | null>(null)

  function spanOf(entry: Entry): { start: number; end: number } {
    if (drag && drag.mode !== 'create' && drag.id === entry.id) {
      return { start: drag.start, end: drag.end }
    }
    const start = Date.parse(entry.start_at)
    return { start, end: entry.end_at ? Date.parse(entry.end_at) : now }
  }

  const blocks = $derived.by(() => {
    if (!msPerPx) return []
    return entries
      .map((entry) => {
        const span = spanOf(entry)
        const bottom = yOf(Math.min(span.end, dayEnd))
        const top = yOf(Math.max(span.start, dayStart))
        return { entry, ...span, top, height: bottom - top }
      })
      .filter((block) => block.height > 0)
      .map((block) => ({ ...block, height: Math.max(6, block.height - 2) }))
  })

  // Projects with time keep their place by first entry; remembered lanes
  // without time sit after them, next to the + column.
  const lanes = $derived.by(() => {
    const ids = orderProjects(
      blocks.map((block) => ({ projectID: block.entry.project_id, start: block.start })),
      laneIDs,
    )
    return ids.map((id, index) => {
      const mine = blocks.filter((block) => block.entry.project_id === id)
      const project = projectByID.get(id)
      return {
        id,
        name: project?.name ?? `Archived project #${id}`,
        color: project?.color || '#64748b',
        x: AXIS_W + index * LANE_W,
        total: formatDuration(
          mine.reduce((sum, block) => sum + (Math.min(block.end, dayEnd) - Math.max(block.start, dayStart)) / 1000, 0),
        ),
        blocks: mine,
      }
    })
  })

  const addX = $derived(AXIS_W + lanes.length * LANE_W)
  const contentWidth = $derived(addX + ADD_W)
  const nowY = $derived(now >= dayStart && now < dayEnd ? yOf(now) : null)
  const hours = $derived.by(() => {
    const out: { label: string; y: number }[] = []
    for (let hour = 0; hour < 24; hour++) {
      const at = wallTimeToInstant(tz, `${day}T${String(hour).padStart(2, '0')}:00`)
      if (at < dayStart || at >= dayEnd) continue
      out.push({ label: `${String(hour).padStart(2, '0')}:00`, y: yOf(at) })
    }
    return out
  })
  const draft = $derived.by(() => {
    const active = drag
    if (!active || active.mode !== 'create' || !active.moved) return null
    const top = yOf(Math.max(active.start, dayStart))
    const bottom = Math.min(yOf(active.end), bodyHeight)
    const index = lanes.findIndex((lane) => lane.id === active.projectID)
    return {
      x: AXIS_W + index * LANE_W,
      top,
      height: Math.max(2, bottom - top),
    }
  })
  const pickable = $derived(
    choices
      .filter((project) => !lanes.some((lane) => lane.id === project.id))
      .filter((project) => project.name.toLowerCase().includes(pickerFilter.trim().toLowerCase())),
  )

  let panelMode = $state<'new' | 'edit' | null>(null)
  let panelEntry = $state<Entry | null>(null)
  let panelProject = $state('')
  let panelStart = $state('')
  let panelEnd = $state('')
  let panelTz = $state(tz)
  let panelNote = $state('')
  const panelRunning = $derived(panelEntry !== null && panelEntry.end_at === null)

  let requestID = 0

  async function load({ keepError = false }: { keepError?: boolean } = {}) {
    if (!userID) return
    const id = ++requestID
    const from = addDays(day, -1)
    const to = addDays(day, 2)
    const { data, error: problem, response } = await api.GET('/api/v1/entries', {
      params: { query: { user_id: userID, from, to } },
    })
    if (id !== requestID) return
    if (problem) error = problemMessage(problem, response)
    else {
      entries = data ?? []
      if (!keepError) error = ''
    }
    loading = false
  }

  async function loadProjects() {
    const { data, error: problem, response } = await api.GET('/api/v1/projects')
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    projects = data ?? []
    const remembered = laneIDs.filter((id) => projects.some((project) => project.id === id))
    laneIDs = remembered
    lastProject = remembered[0] ?? projects.find((project) => !project.archived_at)?.id ?? 0
  }

  function readLanes(): number[] {
    try {
      const stored = JSON.parse(localStorage.getItem(LANES_KEY) ?? '[]')
      return Array.isArray(stored) ? stored.map(Number).filter((id) => Number.isFinite(id) && id > 0) : []
    } catch {
      return []
    }
  }

  function rememberLanes() {
    localStorage.setItem(LANES_KEY, JSON.stringify(laneIDs))
  }

  async function refresh() {
    now = Date.now()
    await load({ keepError: true })
  }

  $effect(() => {
    day
    if (!userID) return
    void load()
  })

  $effect(() => {
    if (panelMode === null) return
    void tick().then(() => document.getElementById('panel-project')?.focus())
  })

  onMount(() => {
    laneIDs = readLanes()
    lastProject = laneIDs[0] ?? 0
    const timer = setInterval(() => (now = Date.now()), 30_000)
    void loadProjects()
    void tick().then(scrollToHour)
    return () => clearInterval(timer)
  })

  // Open about an hour above "now" on today, else at the start of the workday.
  function scrollToHour() {
    const el = scrollEl
    if (!el) return
    const hour = today === day ? Math.max(0, zonedParts(now, tz).hour - 1) : 8
    el.scrollTop = Math.max(0, HEAD_H + hour * HOUR_H - 8)
  }

  function shiftDay(days: number) {
    day = addDays(day, days)
    void tick().then(scrollToHour)
  }

  function goToday() {
    if (day !== today) day = today
    void tick().then(scrollToHour)
  }

  function yAt(event: PointerEvent): number {
    const el = bodyEl
    if (!el) return 0
    return event.clientY - el.getBoundingClientRect().top
  }

  function timeAt(event: PointerEvent): number {
    const at = dayStart + yAt(event) * msPerPx
    return Math.min(Math.max(at, dayStart), dayEnd - MIN)
  }

  function capture(event: PointerEvent) {
    scrollEl?.setPointerCapture(event.pointerId)
  }

  function downCanvas(event: PointerEvent) {
    if (event.button !== 0 || !msPerPx || !bodyEl) return
    if ((event.target as HTMLElement).closest('.lanes-head')) return
    const x = event.clientX - bodyEl.getBoundingClientRect().left
    const lane = lanes[Math.floor((x - AXIS_W) / LANE_W)]
    if (x < AXIS_W || !lane) return
    const projectID = lane.id
    capture(event)
    const anchor = snap(timeAt(event), event.altKey ? FINE_SNAP : SNAP)
    drag = {
      mode: 'create',
      projectID,
      anchor,
      start: anchor,
      end: anchor,
      originY: event.clientY,
      moved: false,
      pointerID: event.pointerId,
    }
    lastProject = projectID
  }

  function downBlock(event: PointerEvent, entry: Entry, mode: 'move' | 'resize-start' | 'resize-end') {
    if (event.button !== 0 || !msPerPx) return
    event.stopPropagation()
    capture(event)
    const span = spanOf(entry)
    drag = {
      mode,
      id: entry.id,
      origStart: span.start,
      origEnd: span.end,
      start: span.start,
      end: span.end,
      originY: event.clientY,
      moved: false,
      // A running entry is retimed by stopping it; dragging would silently stop
      // the timer, so it stays click-only.
      running: entry.end_at === null,
      pointerID: event.pointerId,
    }
    lastProject = entry.project_id
    selectedID = entry.id
  }

  function pointerMove(event: PointerEvent) {
    if (!drag || !msPerPx) return
    if (drag.mode !== 'create' && drag.running) return
    const shift = event.clientY - drag.originY
    if (Math.abs(shift) > 3) drag.moved = true
    const delta = shift * msPerPx
    const step = event.altKey ? FINE_SNAP : SNAP

    if (drag.mode === 'create') {
      const at = snap(timeAt(event), step)
      const range = clampRange(Math.min(drag.anchor, at), Math.max(drag.anchor, at), now)
      drag.start = range.start
      drag.end = range.end
      return
    }
    if (drag.mode === 'move') {
      const start = snap(drag.origStart + delta, step)
      const range = clampRange(start, start + (drag.origEnd - drag.origStart), now, { moving: true })
      drag.start = range.start
      drag.end = range.end
      return
    }
    if (drag.mode === 'resize-start') {
      const range = clampRange(snap(drag.origStart + delta, step), drag.origEnd, now)
      drag.start = range.start
      drag.end = range.end
      return
    }
    const range = clampRange(drag.origStart, snap(drag.origEnd + delta, step), now)
    drag.start = range.start
    drag.end = range.end
  }

  async function pointerUp() {
    const finished = drag
    if (!finished) return
    drag = null

    if (!finished.moved) {
      if (finished.mode === 'create') openNew(finished.anchor, finished.anchor + HOUR, finished.projectID)
      else openEditById(finished.id)
      return
    }

    error = ''
    if (finished.mode === 'create') {
      if (finished.end - finished.start < MIN) {
        openNew(finished.anchor, finished.anchor + HOUR, finished.projectID)
        return
      }
      pending = true
      const { data, error: problem, response } = await api.POST('/api/v1/entries', {
        body: {
          project_id: finished.projectID,
          start_at: new Date(finished.start).toISOString(),
          end_at: new Date(finished.end).toISOString(),
          timezone: tz,
          note: '',
        },
      })
      pending = false
      if (problem) error = problemMessage(problem, response)
      else if (data) openEdit(data)
      await refresh()
      return
    }

    pending = true
    const { data, error: problem, response } = await api.PATCH('/api/v1/entries/{id}', {
      params: { path: { id: finished.id } },
      body: {
        start_at: new Date(finished.start).toISOString(),
        end_at: new Date(finished.end).toISOString(),
      },
    })
    pending = false
    if (problem) error = problemMessage(problem, response)
    else if (data) {
      if (panelMode === 'edit' && panelEntry?.id === data.id) openEdit(data)
      else selectedID = finished.id
    }
    await refresh()
  }

  function cancelDrag() {
    if (!drag) return
    if (scrollEl?.hasPointerCapture(drag.pointerID)) scrollEl.releasePointerCapture(drag.pointerID)
    drag = null
  }

  function openNew(startMs: number, endMs: number, projectID: number) {
    const range = clampRange(snap(startMs, SNAP), snap(endMs, SNAP), now)
    panelMode = 'new'
    panelEntry = null
    panelProject = String(projectID)
    panelTz = tz
    panelStart = instantToWallTime(panelTz, range.start)
    panelEnd = instantToWallTime(panelTz, range.end)
    panelNote = ''
    selectedID = null
  }

  function openEdit(entry: Entry) {
    panelMode = 'edit'
    panelEntry = entry
    panelProject = String(entry.project_id)
    panelTz = entry.timezone || tz
    panelStart = instantToWallTime(panelTz, Date.parse(entry.start_at))
    panelEnd = entry.end_at ? instantToWallTime(panelTz, Date.parse(entry.end_at)) : instantToWallTime(panelTz, now)
    panelNote = entry.note
    selectedID = entry.id
  }

  function openEditById(id: number) {
    const entry = entries.find((candidate) => candidate.id === id)
    if (entry) openEdit(entry)
  }

  function closePanel() {
    panelMode = null
    panelEntry = null
  }

  async function savePanel() {
    if (panelMode === null || pending) return
    const project = Number(panelProject)
    if (!project) {
      error = 'Choose a project first.'
      return
    }
    error = ''
    pending = true
    try {
      if (panelMode === 'new') {
        const range = clampRange(
          wallTimeToInstant(panelTz, panelStart),
          wallTimeToInstant(panelTz, panelEnd),
          now,
        )
        const { data, error: problem, response } = await api.POST('/api/v1/entries', {
          body: {
            project_id: project,
            start_at: new Date(range.start).toISOString(),
            end_at: new Date(range.end).toISOString(),
            timezone: panelTz,
            note: panelNote,
          },
        })
        if (problem) {
          error = problemMessage(problem, response)
          return
        }
        if (data) openEdit(data)
      } else {
        const entry = panelEntry
        if (!entry) return
        const body: components['schemas']['EntryPatchInputBody'] = { note: panelNote }
        if (project !== entry.project_id) body.project_id = project
        // Only send times that the user touched: a note edit must not round an
        // entry with seconds down to the minute.
        const changed =
          panelStart !== instantToWallTime(panelTz, Date.parse(entry.start_at)) ||
          (entry.end_at !== null &&
            panelEnd !== instantToWallTime(panelTz, Date.parse(entry.end_at)))
        if (entry.end_at && changed) {
          const range = clampRange(
            wallTimeToInstant(panelTz, panelStart),
            wallTimeToInstant(panelTz, panelEnd),
            now,
          )
          body.start_at = new Date(range.start).toISOString()
          body.end_at = new Date(range.end).toISOString()
        }
        const { data, error: problem, response } = await api.PATCH('/api/v1/entries/{id}', {
          params: { path: { id: entry.id } },
          body,
        })
        if (problem) {
          error = problemMessage(problem, response)
          return
        }
        if (data) openEdit(data)
      }
      await refresh()
    } finally {
      pending = false
    }
  }

  async function deleteEntry(id: number) {
    if (pending) return
    error = ''
    pending = true
    const { error: problem, response } = await api.DELETE('/api/v1/entries/{id}', {
      params: { path: { id } },
    })
    pending = false
    if (problem) {
      error = problemMessage(problem, response)
      return
    }
    if (panelEntry?.id === id) closePanel()
    if (selectedID === id) selectedID = null
    await refresh()
  }

  function removeSelected() {
    if (selectedID === null) return
    if (!window.confirm('Delete this entry?')) return
    void deleteEntry(selectedID)
  }

  async function startTimer(projectID: number) {
    if (!projectID || pending) return
    error = ''
    pending = true
    const { data, error: problem, response } = await api.POST('/api/v1/timer/start', {
      body: { project_id: projectID, timezone: tz },
    })
    pending = false
    if (problem) error = problemMessage(problem, response)
    else if (data) selectedID = data.id
    await refresh()
  }

  async function stopTimer() {
    if (pending) return
    error = ''
    pending = true
    const { data, error: problem, response } = await api.POST('/api/v1/timer/stop', {})
    pending = false
    if (problem) error = problemMessage(problem, response)
    else if (data) {
      selectedID = data.id
      if (panelEntry?.id === data.id) openEdit(data)
    }
    await refresh()
  }

  function toggleTimer(projectID: number) {
    if (pending) return
    if (running) {
      if (running.project_id === projectID) void stopTimer()
      return
    }
    void startTimer(projectID)
  }

  function openPicker() {
    pickerFilter = ''
    pickerOpen = true
  }

  function addLane(projectID: number) {
    if (!laneIDs.includes(projectID)) {
      laneIDs = [...laneIDs, projectID]
      rememberLanes()
    }
    lastProject = projectID
    pickerOpen = false
    // The new lane is the last one; bring it into view.
    void tick().then(() => {
      if (scrollEl) scrollEl.scrollLeft = scrollEl.scrollWidth
    })
  }

  async function deletePanelEntry() {
    if (panelEntry === null) return
    if (!window.confirm('Delete this entry?')) return
    await deleteEntry(panelEntry.id)
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      if (drag) {
        event.preventDefault()
        cancelDrag()
      } else if (panelMode !== null) {
        event.preventDefault()
        closePanel()
      }
      return
    }
    const target = event.target
    // Let typing and Space activation work inside controls.
    if (target instanceof HTMLElement && target.closest('input, select, textarea, button, a, dialog, wa-input, wa-select, wa-textarea, wa-checkbox, wa-button, wa-dialog')) return
    if (panelMode !== null) return

    if (event.key === ' ') {
      event.preventDefault()
      const project = running?.project_id ?? lastProject ?? lanes[0]?.id ?? 0
      if (project) toggleTimer(project)
    } else if (event.key === 'n' || event.key === 'N') {
      event.preventDefault()
      const end = snap(now, SNAP)
      const project = lastProject || lanes[0]?.id
      if (project) openNew(end - HOUR, end, project)
    } else if (event.key === 'Delete' || event.key === 'Backspace') {
      if (selectedID === null) return
      event.preventDefault()
      removeSelected()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="timeline-page">
  <h1>Timeline</h1>

  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}

  <div class="toolbar">
    <div class="timeline-nav">
      <Button variant="secondary" onclick={() => shiftDay(-1)}>
        <span aria-hidden="true">◀</span><span class="sr-only">Previous day</span>
      </Button>
      <Button variant="secondary" onclick={goToday}>Today</Button>
      <Button variant="secondary" onclick={() => shiftDay(1)} disabled={day >= today}>
        <span aria-hidden="true">▶</span><span class="sr-only">Next day</span>
      </Button>
    </div>
    <span class="day-title">{dayTitle}</span>
    <p class="hint">
      Drag on a lane to log · Alt for 1-minute steps · space start/stop · n new · Delete remove · Esc cancel
    </p>
  </div>

  {#if loading}
    <p class="hint">Loading timeline…</p>
  {/if}

  <div class="timeline-layout" class:with-panel={panelMode !== null}>
    <div
      class="lanes-scroll"
      role="group"
      aria-label="Timeline"
      bind:this={scrollEl}
      onpointerdown={downCanvas}
      onpointermove={pointerMove}
      onpointerup={pointerUp}
      onpointercancel={cancelDrag}
    >
      <div class="lanes" style:width={`${contentWidth}px`}>
        <div class="lanes-head">
          {#each lanes as lane (lane.id)}
            <div class="lane-head" style:left={`${lane.x + 6}px`} style:width={`${LANE_W - 12}px`}>
              <span class="lane-dot" style:background={lane.color} aria-hidden="true"></span>
              <span class="lane-name" title={lane.name}>{lane.name}</span>
              <span class="lane-total">{lane.total}</span>
              <button
                class="lane-timer"
                class:running={running?.project_id === lane.id}
                type="button"
                aria-label={running?.project_id === lane.id ? `Stop timer on ${lane.name}` : `Start timer on ${lane.name}`}
                disabled={running !== null && running.project_id !== lane.id}
                onclick={() => toggleTimer(lane.id)}
              >
                {running?.project_id === lane.id ? '■' : '▶'}
              </button>
            </div>
          {/each}
          <button
            class="add-lane-head"
            type="button"
            style:left={`${addX + 6}px`}
            aria-label="Add project lane"
            onclick={openPicker}
          >
            +
          </button>
        </div>

        <div class="lanes-body" bind:this={bodyEl} style:height={`${bodyHeight}px`}>
          {#each hours as hour (hour.label)}
            <div class="hour-line" style:top={`${hour.y}px`}>
              <span>{hour.label}</span>
            </div>
          {/each}

          {#each lanes as lane (lane.id)}
            <div class="lane" style:left={`${lane.x}px`} style:width={`${LANE_W}px`}></div>
          {/each}

          {#each lanes as lane (lane.id)}
            {#each lane.blocks as block (block.entry.id)}
              <div
                class="entry-block"
                class:selected={selectedID === block.entry.id}
                class:running={!block.entry.end_at}
                role="button"
                tabindex="0"
                aria-label={describe(block.entry, block.start, block.end)}
                title={describe(block.entry, block.start, block.end)}
                style:left={`${lane.x + 6}px`}
                style:width={`${LANE_W - 12}px`}
                style:top={`${block.top}px`}
                style:height={`${block.height}px`}
                style:background={lane.color}
                onpointerdown={(event) => downBlock(event, block.entry, 'move')}
                onkeydown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    openEdit(block.entry)
                  }
                }}
              >
                <span
                  class="resize top"
                  aria-hidden="true"
                  onpointerdown={(event) => downBlock(event, block.entry, 'resize-start')}
                ></span>
                {#if block.height > 26}
                  {#if block.entry.note}
                    <span class="block-text">{block.entry.note}</span>
                  {/if}
                  <span class="block-dur">
                    {formatDuration((block.end - block.start) / 1000)}
                  </span>
                {/if}
                <span
                  class="resize bottom"
                  aria-hidden="true"
                  onpointerdown={(event) => downBlock(event, block.entry, 'resize-end')}
                ></span>
              </div>
            {/each}
          {/each}

          {#if draft}
            <div
              class="draft"
              style:left={`${draft.x + 6}px`}
              style:width={`${LANE_W - 12}px`}
              style:top={`${draft.top}px`}
              style:height={`${draft.height}px`}
            ></div>
          {/if}

          <div class="add-lane" style:left={`${addX}px`} style:width={`${ADD_W}px`} aria-hidden="true"></div>

          {#if lanes.length === 0 && !loading}
            <p class="lanes-empty">Hit + to add a project and start logging.</p>
          {/if}

          {#if nowY !== null}
            <div class="now-line" style:top={`${nowY}px`}>
              <span>{clock.format(now)}</span>
            </div>
          {/if}
        </div>
      </div>
    </div>

    {#if panelMode !== null}
      <aside class="entry-panel" aria-label={panelMode === 'new' ? 'New entry' : 'Edit entry'}>
        <h2>{panelMode === 'new' ? 'New entry' : 'Edit entry'}</h2>
        <form onsubmit={(event) => { event.preventDefault(); void savePanel() }}>
          <Select
            id="panel-project"
            label="Project"
            bind:value={panelProject}
            disabled={pending}
          >
            {#if panelEntry && !projectByID.has(panelEntry.project_id)}
              <wa-option value={String(panelEntry.project_id)}>Archived project</wa-option>
            {/if}
            {#each choices as project (project.id)}
              <wa-option value={String(project.id)}>{project.name}</wa-option>
            {/each}
          </Select>
          <Input
            id="panel-start"
            label="Start"
            type="datetime-local"
            step="60"
            bind:value={panelStart}
            disabled={pending || panelRunning}
          />
          {#if !panelRunning}
            <Input
              id="panel-end"
              label="End"
              type="datetime-local"
              step="60"
              bind:value={panelEnd}
              disabled={pending}
            />
          {/if}
          <Textarea
            id="panel-note"
            label="Note"
            rows={3}
            bind:value={panelNote}
            disabled={pending}
          />
          <div class="row-actions">
            <Button type="submit" disabled={pending}>
              {pending ? 'Saving…' : 'Save'}
            </Button>
            {#if panelMode === 'edit'}
              <Button variant="secondary" onclick={stopTimer} disabled={pending || !panelRunning}>
                Stop now
              </Button>
              <Button variant="danger" onclick={deletePanelEntry} disabled={pending}>
                Delete
              </Button>
            {/if}
            <Button variant="secondary" onclick={closePanel} disabled={pending}>Cancel</Button>
          </div>
          {#if panelRunning}
            <p class="hint">Running since {panelStart.replace('T', ' ')}. Stop it to change the start.</p>
          {/if}
        </form>
      </aside>
    {/if}
  </div>
</div>

<wa-dialog
  class="project-picker"
  label="Add project"
  open={pickerOpen}
  onwa-after-show={() => document.getElementById('picker-search')?.focus()}
  onwa-after-hide={() => (pickerOpen = false)}
>
  <Input
    id="picker-search"
    label="Search projects"
    type="search"
    placeholder="Search projects…"
    bind:value={pickerFilter}
  />
  {#if pickable.length === 0}
    <p class="hint">No projects left to add.</p>
  {:else}
    <ul>
      {#each pickable as project (project.id)}
        <li>
          <button type="button" onclick={() => addLane(project.id)}>
            <span class="lane-dot" style:background={project.color || '#64748b'} aria-hidden="true"></span>
            {project.name}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="row-actions">
    <Button variant="secondary" onclick={() => (pickerOpen = false)}>Cancel</Button>
  </div>
</wa-dialog>
