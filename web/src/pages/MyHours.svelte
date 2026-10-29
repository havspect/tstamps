<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import {
    BarController,
    BarElement,
    CategoryScale,
    Chart,
    Legend,
    LinearScale,
    Tooltip,
  } from 'chart.js'
  import { api, problemMessage } from '../lib/api'
  import type { components } from '../lib/api/schema'
  import { deviceTimezone, formatDuration, todayIn } from '../lib/format'
  import ReportTable from '../lib/ReportTable.svelte'
  import { router } from '../lib/router.svelte'
  import {
    bucketRange,
    chartSeries,
    csvHref,
    pivot,
    presetRange,
    summarize,
    type BucketBy,
    type RangePreset,
    type ReportRow,
  } from '../lib/reports'
  import { session } from '../lib/session.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Input from '../lib/ui/Input.svelte'
  import Select from '../lib/ui/Select.svelte'

  Chart.register(BarController, BarElement, CategoryScale, LinearScale, Tooltip, Legend)

  type Project = components['schemas']['ProjectResponse']

  const user = session.user
  const zone = deviceTimezone()
  const start = presetRange('this-week', zone)

  let preset = $state<RangePreset>('this-week')
  let from = $state(start.from)
  let to = $state(start.to)
  let by = $state<BucketBy>(start.by)
  let rows = $state<ReportRow[]>([])
  let projects = $state<Project[]>([])
  let loading = $state(true)
  let error = $state('')

  let canvas = $state<HTMLCanvasElement>()
  let chart: Chart | undefined

  const buckets = $derived(bucketRange(from, to, by))
  const grouped = $derived(pivot(rows))
  const summary = $derived(summarize(grouped))
  const today = todayIn(zone)
  const colors = $derived(new Map(projects.map((project) => [project.id, project.color])))
  const series = $derived(chartSeries(grouped, buckets, (id) => colors.get(id) || '#9a9a92'))
  const csv = $derived(csvHref({ from, to, teamID: '', userID: user?.id }, router.base))
  const filename = $derived(`my-hours-${from}-to-${to}.csv`)

  function rangeError(left: string, right: string): string {
    const f = Date.parse(`${left}T00:00:00Z`)
    const t = Date.parse(`${right}T00:00:00Z`)
    if (Number.isNaN(f) || Number.isNaN(t)) return 'Choose a valid start and end date.'
    if (t <= f) return 'The end date must be after the start date; the end is exclusive.'
    if (t - f > 366 * 86_400_000) return 'Choose a range of at most 366 days.'
    return ''
  }

  async function load() {
    error = rangeError(from, to)
    if (error) return
    loading = true
    const query: { from: string; to: string; by: BucketBy; user_id?: number } = { from, to, by }
    if (user?.id) query.user_id = user.id

    const [reportRes, projectRes] = await Promise.all([
      api.GET('/api/v1/reports', { params: { query } }),
      api.GET('/api/v1/projects', { params: { query: user?.is_admin ? { all: true } : {} } }),
    ])
    if (reportRes.error) {
      error = problemMessage(reportRes.error, reportRes.response)
      rows = []
    } else {
      rows = reportRes.data?.rows ?? []
    }
    projects = projectRes.data ?? []
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

  // The chart is the table's other face: same buckets, same project order, so
  // the legend reads down the table rows.
  $effect(() => {
    if (!canvas) return
    // A reload with no hours unmounts the canvas; a chart still holding the old
    // element would keep drawing where nobody can see it.
    if (chart && chart.canvas !== canvas) {
      chart.destroy()
      chart = undefined
    }
    const data = { labels: series.labels, datasets: series.datasets }
    if (!chart) {
      const style = getComputedStyle(document.documentElement)
      const muted = style.getPropertyValue('--muted').trim() || '#6b6b63'
      const border = style.getPropertyValue('--border').trim() || '#d9d9d1'
      chart = new Chart(canvas, {
        type: 'bar',
        data,
        options: {
          responsive: true,
          maintainAspectRatio: false,
          interaction: { mode: 'index', intersect: false },
          scales: {
            x: { stacked: true, grid: { display: false }, ticks: { color: muted } },
            y: {
              stacked: true,
              beginAtZero: true,
              ticks: { color: muted, callback: (value) => `${value}h` },
              grid: { color: border },
            },
          },
          plugins: {
            legend: { position: 'bottom', labels: { color: muted, boxWidth: 12, boxHeight: 12 } },
            tooltip: {
              callbacks: {
                label: (item) => `${item.dataset.label}: ${formatDuration(Math.round(Number(item.raw) * 3600))}`,
              },
            },
          },
        },
      })
    } else {
      // Chart.js reads the data in place; reassigning chart.data does not
      // redraw.
      chart.data.labels = data.labels
      chart.data.datasets = data.datasets
      chart.update()
    }
  })

  onDestroy(() => chart?.destroy())

  onMount(() => {
    void load()
  })
</script>

<h1>My hours</h1>

{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

{#if !loading || grouped.length > 0}
  <dl class="kpis" aria-label="My hours summary">
    <div>
      <dt>Total logged</dt>
      <dd>{formatDuration(summary.total)}</dd>
    </div>
    <div>
      <dt>Projects</dt>
      <dd>{grouped.length}<span class="kpi-note">with hours</span></dd>
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
    id="my-hours-range"
    label="Range"
    size="s"
    style="width: 12rem"
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
    <Input id="my-hours-from" label="From" type="date" bind:value={from} required />
    <Input id="my-hours-to" label="To" type="date" bind:value={to} required />
    <Button type="submit" disabled={loading}>Apply</Button>
  {/if}
  <a class="csv" href={csv} download={filename}>Download CSV</a>
</form>

{#if grouped.length > 0}
  <div class="chart">
    <!-- The table below is the accessible copy of these numbers. -->
    <canvas bind:this={canvas} aria-hidden="true"></canvas>
  </div>
{/if}

{#if loading}
  <p>Loading your hours…</p>
{:else if grouped.length === 0}
  <p>No hours in this range.</p>
{:else}
  <ReportTable projects={grouped} {buckets} {colors} {today} people={false} />
{/if}

<style>
  .chart {
    height: 22rem;
    margin: 0 0 var(--space-3);
    padding: var(--space-3);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
</style>
