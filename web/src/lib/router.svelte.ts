// A history router small enough to read: path state plus push/replace and the
// back button. A hosted workspace keeps /w/{slug} as a fixed base while the
// application path below it selects a screen.

function currentPath(): string {
  return location.pathname.replace(/\/+$/, '') || '/'
}

function split(pathname: string): { base: string; path: string } {
  const match = /^\/w\/([^/]+)(\/.*)?$/.exec(pathname)
  if (!match) return { base: '', path: pathname }
  return { base: `/w/${match[1]}`, path: match[2] ?? '/' }
}

const initial = split(currentPath())
const hosted =
  document.querySelector('meta[name="tstamps-hosted"]') !== null || initial.base !== ''

let base = $state(initial.base)
let path = $state(initial.path)

export const router = {
  get hosted(): boolean {
    return hosted
  },
  get base(): string {
    return base
  },
  get path(): string {
    return path
  },
  href(to: string): string {
    return base + (to.startsWith('/') ? to : `/${to}`)
  },
  navigate(to: string, options: { replace?: boolean } = {}): void {
    if (to === path) return
    const target = base + (to.startsWith('/') ? to : `/${to}`)
    if (options.replace) history.replaceState(null, '', target)
    else history.pushState(null, '', target)
    const next = split(currentPath())
    base = next.base
    path = next.path
  },
}

window.addEventListener('popstate', () => {
  const next = split(currentPath())
  base = next.base
  path = next.path
})
