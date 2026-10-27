<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLAnchorAttributes } from 'svelte/elements'
  import { router } from './router.svelte'

  let {
    href,
    class: className = '',
    current = false,
    children,
    ...rest
  }: HTMLAnchorAttributes & { href: string; current?: boolean; children?: Snippet } = $props()

  function follow(event: MouseEvent) {
    // Leave modified clicks to the browser so open-in-new-tab still works.
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    router.navigate(href)
  }
</script>

<a href={router.href(href)} class={className} aria-current={current ? 'page' : undefined} onclick={follow} {...rest}>
  {@render children?.()}
</a>
