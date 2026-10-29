<script lang="ts">
  import { formatDuration } from './format'
  import { driftOf } from './reports'
  import type { Bucket, ReportProject } from './reports'

  let {
    projects,
    buckets,
    colors,
    today,
    people = true,
  }: {
    projects: ReportProject[]
    buckets: Bucket[]
    colors: Map<number, string>
    today: string
    people?: boolean
  } = $props()

  let open = $state<number[]>([])
  const cell = (seconds: number | undefined) => (seconds ? formatDuration(seconds) : '')
  const columns = $derived(
    buckets.map((bucket) => projects.reduce((sum, project) => sum + (project.cells[bucket.key] ?? 0), 0)),
  )
  const grand = $derived(projects.reduce((sum, project) => sum + project.total, 0))

  function toggle(projectID: number) {
    open = open.includes(projectID) ? open.filter((id) => id !== projectID) : [...open, projectID]
  }
</script>

{#snippet projectLabel(project: ReportProject)}
  <span class="swatch" style:background={colors.get(project.project_id) ?? 'var(--muted)'}></span>
  {project.project}
  {#if project.archived}<span class="badge">archived</span>{/if}
{/snippet}

{#snippet hours(logged: number | undefined, planned: number | undefined)}
  {@const shown = driftOf(logged, planned)}
  {shown.value}{#if shown.drift}<span class="drift" class:on-plan={shown.drift === 'on plan'}>{shown.drift}</span>{/if}
{/snippet}

<wa-scroller class="report-scroller">
  <table class="report wa-hover-rows wa-tabular-nums">
    <thead>
      <tr>
        <th scope="col">Project</th>
        {#each buckets as bucket (bucket.key)}
          <th scope="col" class="num" class:today={bucket.key === today}>{bucket.label}</th>
        {/each}
        <th scope="col" class="num">Total</th>
      </tr>
    </thead>
    <tbody>
      {#each projects as project (project.project_id)}
        <tr>
          <th scope="row">
            {#if people}
              <button
                type="button"
                class="project"
                aria-expanded={open.includes(project.project_id)}
                onclick={() => toggle(project.project_id)}
              >
                <span class="caret" aria-hidden="true">{open.includes(project.project_id) ? '▾' : '▸'}</span>
                {@render projectLabel(project)}
              </button>
            {:else}
              <span class="project">{@render projectLabel(project)}</span>
            {/if}
          </th>
          {#each buckets as bucket (bucket.key)}
            <td class="num">{@render hours(project.cells[bucket.key], project.planned[bucket.key])}</td>
          {/each}
          <td class="num total">{@render hours(project.total, project.plannedTotal || undefined)}</td>
        </tr>
        {#if people && open.includes(project.project_id)}
          {#each project.people as person (person.user_id)}
            <tr class="person">
              <th scope="row">{person.user}</th>
              {#each buckets as bucket (bucket.key)}
                <td class="num">{@render hours(person.cells[bucket.key], person.planned[bucket.key])}</td>
              {/each}
              <td class="num total">{@render hours(person.total, person.plannedTotal || undefined)}</td>
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
</wa-scroller>
